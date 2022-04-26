package api

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/skynetlabs/pinner/conf"
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
func (api *API) pinPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	var body SkylinkRequest
	err := json.NewDecoder(req.Body).Decode(&body)
	if err != nil {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	// Create the skylink.
	_, err = api.staticDB.SkylinkCreate(req.Context(), body.Skylink, conf.ServerName)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	// If the skylink already exists, add this server to its list of servers.
	if errors.Contains(err, database.ErrSkylinkExists) {
		err = api.staticDB.SkylinkServerAdd(req.Context(), body.Skylink, conf.ServerName)
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	api.WriteSuccess(w)
}

// unpinPOST informs pinner that a given skylink should no longer be pinned by
// any server.
/*
TODO
 - Mark the skylink for unpinning.
 - Unpin the skylink from the local server and remove the server from the list.
 - Keep the skylink in the DB with the unpinning flag up. This will ensure that
 if the skylink is still pinned to any server and we sweep that server and add
 the skylink to the DB, it will be immediately scheduled for unpinning and it
 will be removed from that server.
 - Change the /ping endpoint to check for this flag and remove it, if raised.
*/
func (api *API) unpinPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteError(w, errors.New("unimplemented"), http.StatusTeapot)
}

// sweepPOST instructs pinner to scan the list of skylinks pinned by skyd and
// update its database.
func (api *API) sweepPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.WriteError(w, errors.New("unimplemented"), http.StatusTeapot)
}
