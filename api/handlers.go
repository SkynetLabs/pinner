package api

import (
	"encoding/json"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/skymodules"
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
	// Create the skylink.
	_, err = api.staticDB.SkylinkCreate(req.Context(), body.Skylink, conf.Conf().ServerName)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, err, http.StatusBadRequest)
		return
	}
	// If the skylink already exists, add this server to its list of servers.
	if errors.Contains(err, database.ErrSkylinkExists) {
		var s database.Skylink
		s, err = api.staticDB.SkylinkFetch(req.Context(), body.Skylink)
		if err != nil {
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		// If the skylink is marked as unpinned we mark is as pinned again
		// because a user just pinned it.
		if s.Unpin {
			err = api.staticDB.SkylinkMarkPinned(req.Context(), body.Skylink)
			if err != nil {
				api.WriteError(w, err, http.StatusInternalServerError)
				return
			}
		}
		err = api.staticDB.SkylinkServerAdd(req.Context(), body.Skylink, conf.Conf().ServerName)
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
	// Validate the skylink.
	var sl skymodules.Skylink
	err = sl.LoadString(body.Skylink)
	if err != nil {
		api.WriteError(w, database.ErrInvalidSkylink, http.StatusBadRequest)
		return
	}
	err = api.staticDB.SkylinkMarkUnpinned(req.Context(), sl.String())
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
