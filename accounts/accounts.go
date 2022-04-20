package accounts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/conf"
)

type (
	// Client allows us to talk to accounts service.
	Client struct {
		staticLogger *logrus.Logger
	}
)

// call makes an HTTP request to `accounts`, deserializes the response body into
// the resp argument and returns the status code and error of the request.
// When the error doesn't come with a response code, we return one that makes
// sense in the context of an HTTP request.
func (ac Client) call(ctx context.Context, method string, endpoint string, resp interface{}) (int, error) {
	url := fmt.Sprintf("http://%s:%s%s", conf.AccountsHost, conf.AccountsPort, endpoint)
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return http.StatusBadRequest, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer func() {
		errBody := res.Body.Close()
		if errBody != nil {
			ac.staticLogger.Debugf("Error while closing request body: %v", err)
		}
	}()
	err = json.NewDecoder(res.Body).Decode(&resp)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	return res.StatusCode, nil
}
