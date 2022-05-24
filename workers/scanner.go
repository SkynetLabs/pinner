package workers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/skyd"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"gitlab.com/NebulousLabs/threadgroup"
	"gitlab.com/SkynetLabs/skyd/build"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.sia.tech/siad/modules"
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

// Handy constants used to improve readability.
const (
	baseSectorRedundancy      = 10
	fanoutRedundancy          = 3
	assumedUploadSpeedInBytes = 1 << 30 / 4 / 8 // 25% of 1Gbps in bytes
)

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
	// SleepBetweenHealthChecks defines the wait time between calls to skyd to
	// check the current health of a given file.
	SleepBetweenHealthChecks = build.Select(
		build.Var{
			Standard: 5 * time.Second,
			Dev:      time.Second,
			Testing:  time.Millisecond,
		}).(time.Duration)

	// sleepBetweenScans defines how often we'll scan the DB for underpinned
	// skylinks.
	sleepBetweenScans = build.Select(build.Var{
		// In production we want to use a prime number of hours, so we can
		// de-sync the scan and the sweeps.
		Standard: 19 * time.Hour,
		Dev:      1 * time.Minute,
		Testing:  100 * time.Millisecond,
	}).(time.Duration)
	// sleepBetweenScansVariation defines how much the sleep between scans will
	// vary between executions
	sleepBetweenScansVariation = build.Select(build.Var{
		// In production we want to use a prime number of hours, so we can
		// de-sync the scan and the sweeps.
		Standard: 2 * time.Hour,
		Dev:      1 * time.Second,
		Testing:  10 * time.Millisecond,
	}).(time.Duration)
)

type (
	// Scanner is a background worker that periodically scans the database for
	// underpinned skylinks. Once an underpinned skylink is found (and it's not
	// being pinned by the local server already), Scanner pins it to the local
	// skyd.
	Scanner struct {
		staticDB         *database.DB
		staticLogger     *logrus.Logger
		staticServerName string
		staticSkydClient skyd.Client
		staticTG         *threadgroup.ThreadGroup

		minPinners int
		mu         sync.Mutex
	}
)

// NewScanner creates a new Scanner instance.
func NewScanner(db *database.DB, logger *logrus.Logger, minPinners int, serverName string, skydClient skyd.Client) *Scanner {
	return &Scanner{
		staticDB:         db,
		staticLogger:     logger,
		minPinners:       minPinners,
		staticServerName: serverName,
		staticSkydClient: skydClient,
		staticTG:         &threadgroup.ThreadGroup{},
	}
}

// Close stops the background worker thread.
func (s *Scanner) Close() error {
	return s.staticTG.Stop()
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
		// Rebuild the cache and watch for service shutdown while doing that.
		res := s.staticSkydClient.RebuildCache()
		select {
		case <-s.staticTG.StopChan():
			return
		case <-res.Ch:
			if res.ExternErr != nil {
				s.staticLogger.Warn(errors.AddContext(res.ExternErr, "failed to rebuild skyd client cache"))
			}
		}
		s.refreshMinPinners()
		s.pinUnderpinnedSkylinks()
		// Sleep between database scans.
		select {
		case <-time.After(SleepBetweenScans()):
		case <-s.staticTG.StopChan():
			s.staticLogger.Trace("Stopping scanner.")
			return
		}
	}
}

// pinUnderpinnedSkylinks loops over all underpinned skylinks and pins them.
func (s *Scanner) pinUnderpinnedSkylinks() {
	s.staticLogger.Trace("Entering pinUnderpinnedSkylinks.")
	defer s.staticLogger.Trace("Exiting  pinUnderpinnedSkylinks.")
	for {
		// Check for service shutdown before talking to the DB.
		select {
		case <-s.staticTG.StopChan():
			s.staticLogger.Trace("Stop channel closed.")
			return
		default:
		}

		skylink, sp, continueScanning := s.findAndPinOneUnderpinnedSkylink()
		if !continueScanning {
			return
		}
		// Block until the pinned skylink becomes healthy or until a timeout.
		s.waitUntilHealthy(skylink, sp)
	}
}

// findAndPinOneUnderpinnedSkylink scans the database for one skylinks which is
// either locked by the current server or underpinned. If it finds such a
// skylink, it pins it to the local skyd. The method returns true until it finds
// no further skylinks to process or until it encounters an unrecoverable error,
// such as bad credentials, dead skyd, etc.
func (s *Scanner) findAndPinOneUnderpinnedSkylink() (skylink skymodules.Skylink, sf skymodules.SiaPath, continueScanning bool) {
	s.staticLogger.Trace("Entering findAndPinOneUnderpinnedSkylink")
	defer s.staticLogger.Trace("Exiting  findAndPinOneUnderpinnedSkylink")

	sl, err := s.staticDB.FindAndLockUnderpinned(context.TODO(), s.staticServerName, s.minPinners)
	if database.IsNoSkylinksNeedPinning(err) {
		return skymodules.Skylink{}, skymodules.SiaPath{}, false
	}
	if err != nil {
		s.staticLogger.Warn(errors.AddContext(err, "failed to fetch underpinned skylink"))
		return skymodules.Skylink{}, skymodules.SiaPath{}, false
	}
	defer func() {
		err = s.staticDB.UnlockSkylink(context.TODO(), sl, s.staticServerName)
		if err != nil {
			s.staticLogger.Debug(errors.AddContext(err, "failed to unlock skylink after trying to pin it"))
		}
	}()

	sf, err = s.staticSkydClient.Pin(sl.String())
	if errors.Contains(err, skyd.ErrSkylinkAlreadyPinned) {
		s.staticLogger.Info(err)
		// The skylink is already pinned locally but it's not marked as such.
		err = s.staticDB.AddServerForSkylink(context.TODO(), sl, s.staticServerName, false)
		if err != nil {
			s.staticLogger.Debug(errors.AddContext(err, "failed to mark as pinned by this server"))
		}
		return skymodules.Skylink{}, skymodules.SiaPath{}, true
	}
	if err != nil && (strings.Contains(err.Error(), "API authentication failed.") ||
		strings.Contains(err.Error(), "connect: connection refused")) {
		err = errors.AddContext(err, fmt.Sprintf("unrecoverable error while pinning '%s'", sl))
		s.staticLogger.Error(err)
		return skymodules.Skylink{}, skymodules.SiaPath{}, false
	}
	if err != nil {
		s.staticLogger.Warn(errors.AddContext(err, fmt.Sprintf("failed to pin '%s'", sl)))
		// Since this is not an unrecoverable error, we'll signal the caller to
		// continue trying to pin other skylinks.
		return skymodules.Skylink{}, skymodules.SiaPath{}, true
	}
	s.staticLogger.Infof("Successfully pinned '%s'", sl)
	err = s.staticDB.AddServerForSkylink(context.TODO(), sl, s.staticServerName, false)
	if err != nil {
		s.staticLogger.Debug(errors.AddContext(err, "failed to mark as pinned by this server"))
	}
	return sl, sf, true
}

// estimateTimeToFull calculates how long we should sleep after pinning the given
// skylink in order to give the renter time to fully upload it before we pin
// another one. It returns a ballpark value.
//
// This method makes some assumptions for simplicity:
// * assumes lazy pinning, meaning that none of the fanout is uploaded
// * all skyfiles are assumed to be large files (base sector + fanout) and the
//	metadata is assumed to fill up the base sector (to err on the safe side)
func (s *Scanner) estimateTimeToFull(skylink skymodules.Skylink) time.Duration {
	meta, err := s.staticSkydClient.Metadata(skylink.String())
	if err != nil {
		err = errors.AddContext(err, "failed to get metadata for skylink")
		s.staticLogger.Error(err)
		build.Critical(err)
		return SleepBetweenPins
	}
	chunkSize := 10 * modules.SectorSizeStandard
	numChunks := meta.Length / chunkSize
	if meta.Length%chunkSize > 0 {
		numChunks++
	}
	// remainingUpload is the amount of data we expect to need to upload until
	// the skyfile reaches full redundancy.
	remainingUpload := numChunks*chunkSize*fanoutRedundancy + (baseSectorRedundancy-1)*modules.SectorSize
	secondsRemaining := remainingUpload / assumedUploadSpeedInBytes
	return time.Duration(secondsRemaining) * time.Second
}

// refreshMinPinners makes sure the local value of min pinners matches the one
// in the database.
func (s *Scanner) refreshMinPinners() {
	mp, err := conf.MinPinners(context.TODO(), s.staticDB)
	if err != nil {
		s.staticLogger.Warn(errors.AddContext(err, "failed to fetch the DB value for min_pinners"))
		return
	}
	s.mu.Lock()
	s.minPinners = mp
	s.mu.Unlock()
}

// waitUntilHealthy blocks until the given skylinks becomes fully healthy or a
// timeout occurs.
func (s *Scanner) waitUntilHealthy(skylink skymodules.Skylink, sp skymodules.SiaPath) {
	deadlineTimer := s.deadline(skylink)
	defer deadlineTimer.Stop()
	ticker := time.NewTicker(SleepBetweenHealthChecks)
	defer ticker.Stop()

	// Wait for the pinned file to become fully healthy.
	for {
		health, err := s.staticSkydClient.FileHealth(sp)
		if err != nil {
			err = errors.AddContext(err, "failed to get sia file's health")
			s.staticLogger.Error(err)
			build.Critical(err)
			break
		}
		if health == 0 {
			// The file is now fully uploaded and healthy.
			break
		}
		select {
		case <-ticker.C:
			s.staticLogger.Tracef("Waiting for '%s' to become fully healthy. Current health: %.2f", skylink, health)
		case <-deadlineTimer.C:
			s.staticLogger.Warnf("Skylink '%s' failed to reach full health within the time limit.", skylink)
			break
		case <-s.staticTG.StopChan():
			return
		}
	}
}

// SleepBetweenScans defines how often we'll scan the DB for underpinned
// skylinks. The returned value varies by +/-sleepBetweenScansVariation and it's
// centered on sleepBetweenScans.
func SleepBetweenScans() time.Duration {
	return sleepBetweenScans - sleepBetweenScansVariation + time.Duration(fastrand.Intn(2*int(sleepBetweenScansVariation)))
}

// deadline calculates how much we are willing to wait for a skylink to be fully
// healthy before giving up. It's twice the expected time, as returned by
// estimateTimeToFull.
func (s *Scanner) deadline(skylink skymodules.Skylink) *time.Timer {
	return time.NewTimer(2 * s.estimateTimeToFull(skylink))
}
