package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

type (
	// HealthGET is the response type of GET /health
	HealthGET struct {
		DBAlive    bool `json:"dbAlive"`
		MinPinners int  `json:"minPinners"`
	}
	// SkylinkRequest describes a request that only provides a skylink.
	SkylinkRequest struct {
		Skylink string
	}
	// SweepPOSTResponse is the response to POST /sweep
	SweepPOSTResponse struct {
		Href string
	}
)

// healthGET returns the status of the service
func (api *API) healthGET(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
	mp, err := conf.MinPinners(req.Context(), api.staticDB)
	var status HealthGET
	status.DBAlive = err == nil
	status.MinPinners = mp
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
// update its database. This call is non-blocking, i.e. it will immediately
// return with a success and it will only start a new sweep if there isn't one
// already running. The response is 202 Accepted and the response body contains
// an endpoint link on which the caller can check the status of the sweep.
func (api *API) sweepPOST(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.latestSweepStatusMu.Lock()
	// If there is no sweep in progress - kick one off.
	if !api.latestSweepStatus.InProgress {
		go api.threadedPerformSweep()
	}
	api.latestSweepStatusMu.Unlock()
	// TODO If we want to be able to uniquely identify sweeps we can issue ids
	//  for them and keep their statuses in a map. This would be the appropriate
	//  RESTful approach. I am not sure we need that because all we care about
	//  is to be able to kick off one and wait for it to end and this
	//  implementations is sufficient for that.
	api.WriteJSONCustomStatus(w, SweepPOSTResponse{"/sweep/status"}, http.StatusAccepted)
}

// sweepStatusGET responds with the status of the latest sweep.
func (api *API) sweepStatusGET(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	api.latestSweepStatusMu.Lock()
	defer api.latestSweepStatusMu.Unlock()
	api.WriteJSON(w, api.latestSweepStatus)
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

// threadedPerformSweep performs the actual sweep operation.
// TODO This can be moved to a separate `sweeper` type which would also hold the
//  status of the latest sweep as well as the relevant mutex. This can
//  streamline the structure of API. I recommend this as a F/U.
func (api *API) threadedPerformSweep() {
	api.latestSweepStatusMu.Lock()
	// Double-check for parallel sweeps.
	if api.latestSweepStatus.InProgress {
		api.latestSweepStatusMu.Unlock()
		return
	}
	// Initialise the status to "a sweep is running".
	api.latestSweepStatus = SweepStatus{
		InProgress: true,
		Error:      nil,
		StartTime:  time.Now().UTC(),
		EndTime:    time.Time{},
	}
	api.latestSweepStatusMu.Unlock()
	// Define an error variable which will represent the success of the scan.
	var err error
	// Ensure that we'll finalize the sweep on returning from this method.
	defer func() {
		api.latestSweepStatusMu.Lock()
		api.latestSweepStatus.InProgress = false
		api.latestSweepStatus.EndTime = time.Now().UTC()
		api.latestSweepStatus.Error = err
		api.latestSweepStatusMu.Unlock()
	}()

	// Perform the actual sweep.
	wg := sync.WaitGroup{}
	wg.Add(1)
	var cacheErr error
	go func() {
		defer wg.Done()
		res := api.staticSkydClient.RebuildCache()
		<-res.Ch
		cacheErr = res.ExternErr
	}()

	// We use an independent context because we are not strictly bound to a
	// specific API call. Also, this operation can take significant amount of
	// time and we don't want it to fail because of a timeout.
	ctx := context.Background()
	dbCtx, cancel := context.WithDeadline(ctx, time.Now().UTC().Add(database.MongoDefaultTimeout))
	defer cancel()

	// Get pinned skylinks from the DB
	dbSkylinks, err := api.staticDB.SkylinksForServer(dbCtx, api.staticServerName)
	if err != nil {
		err = errors.AddContext(err, "failed to fetch skylinks for server")
		return
	}
	wg.Wait()
	if cacheErr != nil {
		err = errors.AddContext(cacheErr, "failed to revuild skyd cache")
		return
	}

	unknown, missing := api.staticSkydClient.DiffPinnedSkylinks(dbSkylinks)

	// Remove all unknown skylink from the database.
	var s skymodules.Skylink
	for _, sl := range unknown {
		s, err = database.SkylinkFromString(sl)
		if err != nil {
			err = errors.AddContext(err, "invalid skylink found in DB")
			build.Critical(err)
			return
		}
		err = api.staticDB.RemoveServerFromSkylink(ctx, s, api.staticServerName)
		if err != nil {
			err = errors.AddContext(err, "failed to unpin skylink")
			return
		}
	}
	// Add all missing skylinks to the database.
	for _, sl := range missing {
		s, err = database.SkylinkFromString(sl)
		if err != nil {
			err = errors.AddContext(err, "invalid skylink reported by skyd")
			build.Critical(err)
			return
		}
		err = api.staticDB.AddServerForSkylink(ctx, s, api.staticServerName, false)
		if err != nil {
			err = errors.AddContext(err, "failed to unpin skylink")
			return
		}
	}
}
