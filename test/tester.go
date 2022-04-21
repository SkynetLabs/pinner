package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/api"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
)

var (
	testPortalAddr = "http://127.0.0.1"
	testPortalPort = "6000"

	// dontFollowRedirectsCheckRedirectFn is a function that instructs http.Client
	// to return with the last user response, instead of following a redirect.
	dontFollowRedirectsCheckRedirectFn = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
)

type (
	// Tester is a simple testing kit. It starts a testing instance of the
	// service and provides simplified ways to call the handlers.
	Tester struct {
		Ctx             context.Context
		DB              *database.DB
		FollowRedirects bool
		Logger          *logrus.Logger

		cancel context.CancelFunc
	}
)

// NewDatabase returns a new DB connection based on the passed parameters.
func NewDatabase(ctx context.Context, dbName string) (*database.DB, error) {
	return database.NewCustomDB(ctx, SanitizeName(dbName), DBTestCredentials(), NewDiscardLogger())
}

// NewTester creates and starts a new Tester service.
// Use the Close method for a graceful shutdown.
func NewTester(dbName string) (*Tester, error) {
	ctx := context.Background()
	logger := NewDiscardLogger()

	// Connect to the database.
	db, err := NewDatabase(ctx, dbName)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to the DB")
	}

	ctxWithCancel, cancel := context.WithCancel(ctx)

	// The server API encapsulates all the modules together.
	server, err := api.New(db, logger)
	if err != nil {
		cancel()
		return nil, errors.AddContext(err, "failed to build the API")
	}

	// Start the HTTP server in a goroutine and gracefully stop it once the
	// cancel function is called and the context is closed.
	srv := &http.Server{
		Addr:    ":" + testPortalPort,
		Handler: server,
	}
	go func() {
		_ = srv.ListenAndServe()
	}()
	go func() {
		select {
		case <-ctxWithCancel.Done():
			_ = srv.Shutdown(context.TODO())
		}
	}()

	at := &Tester{
		Ctx:             ctxWithCancel,
		DB:              db,
		FollowRedirects: true,
		Logger:          logger,
		cancel:          cancel,
	}
	// Wait for the accounts tester to be fully ready.
	err = build.Retry(50, time.Millisecond, func() error {
		_, _, e := at.HealthGET()
		return e
	})
	if err != nil {
		return nil, errors.AddContext(err, "failed to start accounts tester in the given time")
	}
	return at, nil
}

// NewDiscardLogger returns a new logger that sends all output to ioutil.Discard.
func NewDiscardLogger() *logrus.Logger {
	logger := logrus.New()
	logger.Out = ioutil.Discard
	return logger
}

// SanitizeName sanitizes the input for all kinds of unwanted characters and
// replaces those with underscores.
// See https://docs.mongodb.com/manual/reference/limits/#naming-restrictions
func SanitizeName(s string) string {
	re := regexp.MustCompile(`[/\\.\s"$*<>:|?]`)
	cleanDBName := re.ReplaceAllString(s, "_")
	// 64 characters is MongoDB's limit on database names.
	// See https://docs.mongodb.com/manual/reference/limits/#mongodb-limit-Length-of-Database-Names
	if len(cleanDBName) > 64 {
		cleanDBName = cleanDBName[:64]
	}
	return cleanDBName
}

// Close performs a graceful shutdown of the Tester service.
func (at *Tester) Close() error {
	at.cancel()
	if at.DB != nil {
		err := at.DB.Disconnect(at.Ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetFollowRedirects configures the tester to either follow HTTP redirects or
// not. The default is to follow them.
func (at *Tester) SetFollowRedirects(f bool) {
	at.FollowRedirects = f
}

// post executes a POST Request against the test service.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *Tester) post(endpoint string, params url.Values, bodyParams url.Values) (*http.Response, []byte, error) {
	if params == nil {
		params = url.Values{}
	}
	bodyMap := make(map[string]string)
	for k, v := range bodyParams {
		if len(v) == 0 {
			continue
		}
		bodyMap[k] = v[0]
	}
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return &http.Response{}, nil, err
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + params.Encode()
	req, err := http.NewRequest(http.MethodPost, serviceURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return &http.Response{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return at.executeRequest(req)
}

// Request is a helper method that puts together and executes an HTTP
// Request. It attaches the current cookie, if one exists.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *Tester) Request(method string, endpoint string, queryParams url.Values, body []byte, headers map[string]string, obj interface{}) (*http.Response, error) {
	if queryParams == nil {
		queryParams = url.Values{}
	}
	serviceURL := testPortalAddr + ":" + testPortalPort + endpoint + "?" + queryParams.Encode()
	req, err := http.NewRequest(method, serviceURL, bytes.NewBuffer(body))
	if err != nil {
		return &http.Response{StatusCode: http.StatusInternalServerError}, err
	}
	for name, val := range headers {
		req.Header.Set(name, val)
	}
	r, b, err := at.executeRequest(req)
	// Define a list of response codes we assume are "good". We are going to
	// return an error if the response returns a code that's not on this list.
	acceptedResponseCodes := map[int]bool{
		http.StatusOK:        true,
		http.StatusNoContent: true,
	}
	// Use the response's body as error response on bad response codes.
	if !acceptedResponseCodes[r.StatusCode] {
		if err == nil {
			return r, errors.New(string(b))
		}
		return r, errors.AddContext(err, string(b))
	}
	if r.StatusCode == http.StatusOK {
		// Process the body
		err = json.Unmarshal(b, &obj)
		if err != nil {
			return r, errors.AddContext(err, "failed to marshal the body JSON")
		}
	}
	return r, err
}

// executeRequest is a helper method which executes a test Request and processes
// the response by extracting the body from it and handling non-OK status codes.
//
// NOTE: The Body of the returned response is already read and closed.
func (at *Tester) executeRequest(req *http.Request) (*http.Response, []byte, error) {
	if req == nil {
		return &http.Response{}, nil, errors.New("invalid Request")
	}
	client := http.Client{}
	if !at.FollowRedirects {
		client.CheckRedirect = dontFollowRedirectsCheckRedirectFn
	}
	r, err := client.Do(req)
	if err != nil {
		return &http.Response{}, nil, err
	}
	return processResponse(r)
}

// processResponse is a helper method which extracts the body from the response
// and handles non-OK status codes.
//
// NOTE: The Body of the returned response is already read and closed.
func processResponse(r *http.Response) (*http.Response, []byte, error) {
	body, err := ioutil.ReadAll(r.Body)
	_ = r.Body.Close()
	// For convenience, whenever we have a non-OK status we'll wrap it in an
	// error.
	if r.StatusCode < 200 || r.StatusCode > 299 {
		err = errors.Compose(err, errors.New(r.Status))
	}
	return r, body, err
}

// HealthGET checks the health of the service.
func (at *Tester) HealthGET() (api.HealthGET, int, error) {
	var resp api.HealthGET
	r, err := at.Request(http.MethodGet, "/health", nil, nil, nil, &resp)
	return resp, r.StatusCode, err
}

// PinPOST tells pinner that the current server is pinning a given skylink.
func (at *Tester) PinPOST(sl string) (int, error) {
	body, err := json.Marshal(api.SkylinkRequest{
		Skylink: sl,
	})
	if err != nil {
		return http.StatusBadRequest, errors.AddContext(err, "unable to marshal request body")
	}
	r, err := at.Request(http.MethodPost, "/pin", nil, body, nil, nil)
	return r.StatusCode, err
}
