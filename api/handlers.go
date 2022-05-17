package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/julienschmidt/httprouter"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
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
	sl, err := api.parseAndResolve(body.Skylink)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, database.ErrInvalidSkylink, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	// Create the skylink.
	_, err = api.staticDB.CreateSkylink(req.Context(), sl, api.staticServerName)
	// If the skylink already exists, add this server to its list of servers and
	// mark the skylink as pinned.
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
	sl, err := api.parseAndResolve(body.Skylink)
	if errors.Contains(err, database.ErrInvalidSkylink) {
		api.WriteError(w, database.ErrInvalidSkylink, http.StatusBadRequest)
		return
	}
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
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
// update its database. This is a very heavy call! Use with caution!
func (api *API) sweepPOST(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {

	// Rebuild cache in a goroutine

	wg := sync.WaitGroup{}
	wg.Add(1)

	var cacheErr error
	go func() {
		defer wg.Done()
		cacheErr = api.staticSkydClient.RebuildCache()
	}()

	// Get pinned skylinks from the DB
	dbSkylinks, err := api.staticDB.SkylinksForServer(req.Context(), api.staticServerName)
	if err != nil {
		api.WriteError(w, err, http.StatusInternalServerError)
		return
	}
	wg.Wait()
	if cacheErr != nil {
		api.WriteError(w, cacheErr, http.StatusInternalServerError)
		return
	}

	unknown, missing := api.staticSkydClient.DiffPinnedSkylinks(dbSkylinks)

	ctx := req.Context()
	// Remove all unknown skylink from the database.
	for _, sl := range unknown {
		s, err := database.SkylinkFromString(sl)
		if err != nil {
			err = errors.AddContext(err, "invalid skylink found in DB")
			build.Critical(err)
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		err = api.staticDB.RemoveServerFromSkylink(ctx, s, api.staticServerName)
		if err != nil {
			api.WriteError(w, errors.AddContext(err, "failed to unpin skylink"), http.StatusInternalServerError)
			return
		}
	}
	// Add all missing skylinks to the database.
	for _, sl := range missing {
		s, err := database.SkylinkFromString(sl)
		if err != nil {
			err = errors.AddContext(err, "invalid skylink reported by skyd")
			build.Critical(err)
			api.WriteError(w, err, http.StatusInternalServerError)
			return
		}
		err = api.staticDB.AddServerForSkylink(ctx, s, api.staticServerName, false)
		if err != nil {
			api.WriteError(w, errors.AddContext(err, "failed to unpin skylink"), http.StatusInternalServerError)
			return
		}
	}

	api.WriteSuccess(w)
}

// parseAndResolve parses the given string representation of a skylink and
// resolves it to a V1 skylink, in case it's a V2.
func (api *API) parseAndResolve(skylink string) (skymodules.Skylink, error) {
	var sl skymodules.Skylink
	err := sl.LoadString(skylink)
	if err != nil {
		return skymodules.Skylink{}, errors.Compose(err, database.ErrInvalidSkylink)
	}
	if sl.IsSkylinkV2() {
		s, err := api.staticSkydClient.Resolve(sl.String())
		if err != nil {
			return skymodules.Skylink{}, err
		}
		err = sl.LoadString(s)
		if err != nil {
			return skymodules.Skylink{}, err
		}
	}
	return sl, nil
}
