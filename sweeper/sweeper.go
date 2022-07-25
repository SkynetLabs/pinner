package sweeper

import (
	"context"
	"sync"
	"time"

	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/logger"
	"github.com/skynetlabs/pinner/skyd"
	"gitlab.com/NebulousLabs/errors"
)

const (
	// SweepInterval determines how often we perform our regular sweeps.
	SweepInterval = 24 * time.Hour
)

type (
	// Status represents the status of a sweep.
	Status struct {
		InProgress bool
		Error      error
		StartTime  time.Time
		EndTime    time.Time
	}
	// status is the internal type we use when we want to be able to modify it.
	status struct {
		Status
		mu           sync.Mutex
		staticLogger *logger.Logger
	}
	// Sweeper takes care of sweeping the files pinned by the local skyd server
	// and marks them as pinned by the local server.
	Sweeper struct {
		staticDB         *database.DB
		staticLogger     *logger.Logger
		staticSchedule   *schedule
		staticServerName string
		staticSkydClient skyd.Client
		staticStatus     *status
	}
)

// New returns a new Sweeper.
func New(db *database.DB, skydc skyd.Client, serverName string, logger *logger.Logger) *Sweeper {
	return &Sweeper{
		staticDB:         db,
		staticLogger:     logger,
		staticSchedule:   &schedule{},
		staticServerName: serverName,
		staticSkydClient: skydc,
		staticStatus: &status{
			staticLogger: logger,
		},
	}
}

// Status returns a copy of the status of the current sweep.
func (s *Sweeper) Status() Status {
	st := (*s.staticStatus).Status
	return st
}

// Sweep starts a new skyd sweep, unless one is already underway.
func (s *Sweeper) Sweep() {
	go s.threadedPerformSweep()
}

// UpdateSchedule schedules a new series of sweeps to be run.
// If there are already scheduled sweeps, that schedule is cancelled (running
// sweeps are not interrupted) and a new schedule is established.
func (s *Sweeper) UpdateSchedule(period time.Duration) {
	s.staticSchedule.Update(period, s)
}

// threadedPerformSweep performs the actual sweep operation.
func (s *Sweeper) threadedPerformSweep() {
	if s.staticStatus.InProgress {
		s.staticLogger.Debug("Attempted to start a sweep while another one was already ongoing.")
		return
	}
	// Mark a sweep as started.
	s.staticStatus.Start()
	// Define an error variable which will represent the success of the scan.
	var err error
	// Ensure that we'll finalize the sweep on returning from this method.
	defer func() {
		if err != nil {
			s.staticLogger.Debug(errors.AddContext(err, "sweeping failed with error"))
		}
		s.staticStatus.Finalize(err)
	}()

	// Perform the actual sweep.
	// Kick off a skyd client cache rebuild. That happens in a separate
	// goroutine. We'll block on the result channel only after we're done with
	// the other tasks we can do while waiting.
	res := s.staticSkydClient.RebuildCache()

	// We use an independent context because we are not strictly bound to a
	// specific API call. Also, this operation can take a significant amount of
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
	// Block until the cache rebuild is done.
	<-res.ErrAvail
	if res.ExternErr != nil {
		err = errors.AddContext(res.ExternErr, "failed to rebuild skyd cache")
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

// Start marks the start of a new process, unless one is already in progress.
// If there is a process in progress then Start returns without any action.
func (st *status) Start() {
	st.mu.Lock()
	// Double-check for parallel sweeps.
	if st.InProgress {
		st.mu.Unlock()
		st.staticLogger.Debug("Attempted to start a sweep while another one was already ongoing.")
		return
	}
	// Initialise the status to "a sweep is running".
	st.InProgress = true
	st.Error = nil
	st.StartTime = time.Now().UTC()
	st.EndTime = time.Time{}
	st.mu.Unlock()
	st.staticLogger.Info("Started a sweep.")
}

// Finalize marks a run as completed with the given error.
func (st *status) Finalize(err error) {
	st.mu.Lock()
	st.InProgress = false
	st.EndTime = time.Now().UTC()
	st.Error = err
	st.mu.Unlock()
	st.staticLogger.Info("Finalized a sweep.")
}
