package workers

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/skyd"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/threadgroup"
	"gitlab.com/SkynetLabs/skyd/build"
)

/**
 PHASE 1: <DONE>
 - scan the DB once a day for underpinned files
 - when you find an underpinned file that's not currently locked
	- lock it
	- pin it locally and add the current server to its list
	- unlock it

 PHASE 2:
 - calculate server load by getting the total number and size of files pinned by each server
 - only pin underpinned files if the current server is in the lowest 20% of servers, otherwise exit before scanning further

 PHASE 3:
 - add a second scanner which looks for skylinks which should be unpinned and unpins them from the local skyd.
*/

var (
	// SleepBetweenPins defines how long we'll sleep between pinning files.
	// We want to add this sleep in order to prevent a single server from
	// grabbing all underpinned files and overloading itself. We also want to
	// allow for some time for the newly pinned files to reach full redundancy
	// before we pin more files.
	SleepBetweenPins = build.Select(
		build.Var{
			Standard: 10 * time.Second,
			Dev:      time.Second,
			Testing:  time.Millisecond,
		}).(time.Duration)

	// SleepBetweenScans defines how often we'll scan the DB for underpinned
	// skylinks.
	SleepBetweenScans = build.Select(build.Var{
		// In production we want to use a prime number of hours, so we can
		// de-sync the scan and the sweeps.
		Standard: 19 * time.Hour,
		Dev:      10 * time.Second,
		Testing:  100 * time.Millisecond,
	}).(time.Duration)
)

type (
	// Scanner is a background worker that periodically scans the database for
	// underpinned skylinks. Once an underpinned skylink is found (and it's not
	// being pinned by the local server already), Scanner pins it to the local
	// skyd.
	Scanner struct {
		staticDB            *database.DB
		staticLogger        *logrus.Logger
		staticMinNumPinners int
		staticServerName    string
		staticSkydClient    skyd.Client
		staticTG            *threadgroup.ThreadGroup
	}
)

// NewScanner creates a new Scanner instance.
func NewScanner(db *database.DB, logger *logrus.Logger, minNumPinners int, serverName string, skydClient skyd.Client, tg *threadgroup.ThreadGroup) *Scanner {
	return &Scanner{
		staticDB:            db,
		staticLogger:        logger,
		staticMinNumPinners: minNumPinners,
		staticServerName:    serverName,
		staticSkydClient:    skydClient,
		staticTG:            tg,
	}
}

// Start launches the background worker thread that scans the DB for underpinned
// skylinks.
func (s *Scanner) Start() error {
	err := s.staticTG.Add()
	if err != nil {
		return err
	}

	go s.threadedScanAndPin()

	return nil
}

// threadedScanAndPin defines the scanning operation of Scanner.
func (s *Scanner) threadedScanAndPin() {
	defer s.staticTG.Done()

	// Main execution loop, goes on forever while the service is running.
	for {
		s.pinUnderpinnedSkylinks()
		// Sleep between database scans.
		select {
		case <-time.After(SleepBetweenScans):
		case <-s.staticTG.StopChan():
			return
		}
	}
}

// pinUnderpinnedSkylinks loops over all underpinned skylinks and pin them.
func (s *Scanner) pinUnderpinnedSkylinks() {
	for s.findAndPinOneUnderpinnedSkylink() {
		// Sleep for a bit after pinning before continuing with the next
		// skylink.
		select {
		case <-time.After(SleepBetweenPins):
		case <-s.staticTG.StopChan():
			return
		}
	}
}

// findAndPinOneUnderpinnedSkylink scans the database for one skylinks which is
// either locked by the current server or underpinned. If it finds such a
// skylink, it pins it to the local skyd. The method returns true until it finds
// no further skylinks to process.
func (s *Scanner) findAndPinOneUnderpinnedSkylink() bool {
	sl, err := s.staticDB.FindAndLockUnderpinned(context.TODO(), s.staticServerName, s.staticMinNumPinners)
	if errors.Contains(err, database.ErrSkylinkNoExist) {
		// No more underpinned skylinks pinnable by this server.
		return false
	}
	if err != nil {
		s.staticLogger.Warn(errors.AddContext(err, "failed to fetch underpinned skylink"))
		return true
	}
	defer func() {
		err = s.staticDB.UnlockSkylink(context.TODO(), sl, s.staticServerName)
		if err != nil {
			s.staticLogger.Debug(errors.AddContext(err, "failed to unlock skylink after trying to pin it"))
		}
	}()
	err = s.staticSkydClient.Pin(sl.String())
	if err != nil {
		s.staticLogger.Warn(errors.AddContext(err, "failed to pin skylink"))
		return true
	}
	s.staticLogger.Debugf("Successfully pinned %s", sl)
	return true
}
