package api

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/skynetlabs/pinner/database"
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
// If the skylink already exists and it's marked for unpinning, this method will
// unmark it.
func (api *API) pinPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body SkylinkRequest
	err := json.NewDecoder(req.Body).Decode(&body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	sl, err := database.SkylinkFromString(body.Skylink)
	if err != nil {
		api.WriteError(w, database.ErrInvalidSkylink, http.StatusBadRequest)
		return
	}
	// Create the skylink.
	_, err = api.staticDB.CreateSkylink(req.Context(), sl, api.staticServerName)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	// If the skylink already exists, add this server to its list of servers and
	// mark the skylink as not unpinned.
	if errors.Contains(err, database.ErrSkylinkExists) {
		err = api.staticDB.AddServerForSkylink(req.Context(), sl, api.staticServerName, true)
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// unpinPOST informs pinner that a given skylink should no longer be pinned by
// any server.
func (api *API) unpinPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body SkylinkRequest
	err := json.NewDecoder(req.Body).Decode(&body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	sl, err := database.SkylinkFromString(body.Skylink)
	if err != nil {
		api.WriteError(w, database.ErrInvalidSkylink, http.StatusBadRequest)
		return
	}
	err = api.staticDB.MarkUnpinned(req.Context(), sl)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// sweepPOST instructs pinner to scan the list of skylinks pinned by skyd and
// update its database.
func (api *API) sweepPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteError(w, errors.New("unimplemented"), http.StatusTeapot)
}
