package sweeper

import (
	"context"
	"sync"
	"time"

	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/skyd"
	"gitlab.com/NebulousLabs/errors"
)

type (
	// Status represents the status of a sweep.
	Status struct {
		InProgress bool
		Error      error
		StartTime  time.Time
		EndTime    time.Time
	}
	// Sweeper takes care of sweeping the files pinned by the local skyd server
	// and marks them as pinned by the local server.
	Sweeper struct {
		staticDB         *database.DB
		staticSchedule   *schedule
		staticServerName string
		staticSkydClient skyd.Client

		status   Status
		statusMu sync.Mutex
	}
)

// New returns a new Sweeper.
func New(db *database.DB, skydc skyd.Client, serverName string) *Sweeper {
	return &Sweeper{
		staticDB:         db,
		staticSkydClient: skydc,
		staticServerName: serverName,
		staticSchedule:   &schedule{},
	}
}

// Status returns a copy of the status of the current sweep.
func (s *Sweeper) Status() Status {
	s.statusMu.Lock()
	st := s.status
	s.statusMu.Unlock()
	return st
}

// Sweep starts a new skyd sweep, unless one is already underway.
//
// TODO If we want to be able to uniquely identify sweeps we can issue ids
//  for them and keep their statuses in a map. This would be the appropriate
//  RESTful approach. I am not sure we need that because all we care about
//  is to be able to kick off one and wait for it to end and this
//  implementations is sufficient for that.
func (s *Sweeper) Sweep() {
	go s.threadedPerformSweep()
}

// UpdateSchedule schedules a new series of sweeps to be run.
// If there are already scheduled sweeps, that schedule is cancelled (running
// // sweeps are not interrupted) and a new schedule is established.
func (s *Sweeper) UpdateSchedule(period time.Duration) {
	s.staticSchedule.Update(period, s)
}

// threadedPerformSweep performs the actual sweep operation.
func (s *Sweeper) threadedPerformSweep() {
	s.statusMu.Lock()
	// Double-check for parallel sweeps.
	if s.status.InProgress {
		s.statusMu.Unlock()
		return
	}
	// Initialise the status to "a sweep is running".
	s.status = Status{
		InProgress: true,
		Error:      nil,
		StartTime:  time.Now().UTC(),
		EndTime:    time.Time{},
	}
	s.statusMu.Unlock()
	// Define an error variable which will represent the success of the scan.
	var err error
	// Ensure that we'll finalize the sweep on returning from this method.
	defer func() {
		s.statusMu.Lock()
		s.status.InProgress = false
		s.status.EndTime = time.Now().UTC()
		s.status.Error = err
		s.statusMu.Unlock()
	}()

	// Perform the actual sweep.
	wg := sync.WaitGroup{}
	wg.Add(1)
	var cacheErr error
	go func() {
		defer wg.Done()
		res := s.staticSkydClient.RebuildCache()
		<-res.ErrAvail
		cacheErr = res.ExternErr
	}()

	// We use an independent context because we are not strictly bound to a
	// specific API call. Also, this operation can take significant amount of
	// time and we don't want it to fail because of a timeout.
	ctx := context.Background()
	dbCtx, cancel := context.WithDeadline(ctx, time.Now().UTC().Add(database.MongoDefaultTimeout))
	defer cancel()

	// Get pinned skylinks from the DB
	dbSkylinks, err := s.staticDB.SkylinksForServer(dbCtx, s.staticServerName)
	if err != nil {
		err = errors.AddContext(err, "failed to fetch skylinks for server")
		return
	}
	wg.Wait()
	if cacheErr != nil {
		err = errors.AddContext(cacheErr, "failed to rebuild skyd cache")
		return
	}

	unknown, missing := s.staticSkydClient.DiffPinnedSkylinks(dbSkylinks)
	// Remove all unknown skylinks from the database.
	err = s.staticDB.RemoveServerFromSkylinks(ctx, unknown, s.staticServerName)
	if err != nil {
		err = errors.AddContext(err, "failed to remove server for skylink")
		return
	}
	// Add all missing skylinks to the database.
	err = s.staticDB.AddServerForSkylinks(ctx, missing, s.staticServerName, false)
	if err != nil {
		err = errors.AddContext(err, "failed to add server for skylink")
		return
	}
}
