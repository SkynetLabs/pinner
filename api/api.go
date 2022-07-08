package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/logger"
	"github.com/skynetlabs/pinner/skyd"
	"github.com/skynetlabs/pinner/sweeper"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
)

type (
	// API is the central struct which gives us access to all subsystems.
	API struct {
		staticServerName string
		staticDB         *database.DB
		staticLogger     logger.ExtFieldLogger
		staticRouter     *httprouter.Router
		staticSkydClient skyd.Client
		staticSweeper    *sweeper.Sweeper
	}

	// errorWrap is a helper type for converting an `error` struct to JSON.
	errorWrap struct {
		Message string `json:"message"`
	}
)

// New returns a new initialised API.
func New(serverName string, db *database.DB, logger logger.ExtFieldLogger, skydClient skyd.Client, sweeper *sweeper.Sweeper) (*API, error) {
	if db == nil {
		return nil, errors.New("no DB provided")
	}
	if logger == nil {
		return nil, errors.New("invalid logger provided")
	}
	router := httprouter.New()
	router.RedirectTrailingSlash = true

	apiInstance := &API{
		staticServerName: serverName,
		staticDB:         db,
		staticLogger:     logger,
		staticRouter:     router,
		staticSkydClient: skydClient,
		staticSweeper:    sweeper,
	}
	apiInstance.buildHTTPRoutes()
	return apiInstance, nil
}

// ServeHTTP implements the http.Handler interface.
func (api *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	api.staticRouter.ServeHTTP(w, req)
}

// ListenAndServe starts the API server on the given port.
func (api *API) ListenAndServe(port int) error {
	api.staticLogger.Info(fmt.Sprintf("Listening on port %d", port))
	return http.ListenAndServe(fmt.Sprintf(":%d", port), api.staticRouter)
}

// WriteError an error to the API caller.
func (api *API) WriteError(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	api.staticLogger.Errorln(code, err)
	encodingErr := json.NewEncoder(w).Encode(errorWrap{Message: err.Error()})
	if _, isJSONErr := encodingErr.(*json.SyntaxError); isJSONErr {
		// Marshalling should only fail in the event of a developer error.
		// Specifically, only non-marshallable types should cause an error here.
		build.Critical("failed to encode API error response:", encodingErr)
	}
}

// WriteJSON writes the object to the ResponseWriter. If the encoding fails, an
// error is written instead. The Content-Type of the response header is set
// accordingly.
func (api *API) WriteJSON(w http.ResponseWriter, obj interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	api.staticLogger.Traceln(http.StatusOK)
	err := json.NewEncoder(w).Encode(obj)
	if err != nil {
		api.staticLogger.Debugln(err)
	}
	if _, isJSONErr := err.(*json.SyntaxError); isJSONErr {
		// Marshalling should only fail in the event of a developer error.
		// Specifically, only non-marshallable types should cause an error here.
		build.Critical("failed to encode API response:", err)
	}
}

// WriteJSONCustomStatus writes the object to the ResponseWriter. If the
// encoding fails, an error is written instead. The Content-Type of the response
// header is set accordingly.
func (api *API) WriteJSONCustomStatus(w http.ResponseWriter, obj interface{}, status int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	api.staticLogger.Traceln(status)
	err := json.NewEncoder(w).Encode(obj)
	if err != nil {
		api.staticLogger.Debugln(err)
	}
	if _, isJSONErr := err.(*json.SyntaxError); isJSONErr {
		// Marshalling should only fail in the event of a developer error.
		// Specifically, only non-marshallable types should cause an error here.
		build.Critical("failed to encode API response:", err)
	}
}

// WriteSuccess writes the HTTP header with status 204 No Content to the
// ResponseWriter. WriteSuccess should only be used to indicate that the
// requested action succeeded AND there is no data to return.
func (api *API) WriteSuccess(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
	api.staticLogger.Traceln(http.StatusNoContent)
}
