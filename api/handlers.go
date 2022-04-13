package api

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"gitlab.com/NebulousLabs/errors"
)

type (
	// HealthGET is the response type of GET /health
	HealthGET struct {
		DBAlive bool `json:"dbAlive"`
	}
	// SkylinkRequest describes a request that only provides a skylink.
	SkylinkRequest struct {
		Skylink string
	}
)

// healthGET returns the status of the service
func (api *API) healthGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var status HealthGET
	err := api.staticDB.Ping(req.Context())
	status.DBAlive = err == nil
	api.WriteJSON(w, status)
}

// pinPOST informs pinner that a given skylink is pinned on the current server.
func (api *API) pinPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteError(w, errors.New("unimplemented"), http.StatusTeapot)
}

// unpinPOST informs pinner that a given skylink is pinned on the current server.
func (api *API) unpinPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteError(w, errors.New("unimplemented"), http.StatusTeapot)
}

// sweepPOST instructs pinner to scan the list of skylinks pinned by skyd and
// update its database.
func (api *API) sweepPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteError(w, errors.New("unimplemented"), http.StatusTeapot)
}
