package workers

import (
	"context"
	"math"
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

// Handy constants used to improve readability.
const (
	sectorSize                = 1 << 22         // 4 MiB
	chunkSize                 = 10 * sectorSize // 40 MiB
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
		staticDB         *database.DB
		staticLogger     *logrus.Logger
		staticMinPinners int
		staticServerName string
		staticSkydClient skyd.Client
		staticTG         *threadgroup.ThreadGroup
	}
)

// NewScanner creates a new Scanner instance.
func NewScanner(db *database.DB, logger *logrus.Logger, minPinners int, serverName string, skydClient skyd.Client) *Scanner {
	return &Scanner{
		staticDB:         db,
		staticLogger:     logger,
		staticMinPinners: minPinners,
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

	for {
		skylink, continueScanning := s.findAndPinOneUnderpinnedSkylink()
		if !continueScanning {
			return
		}
		// Sleep for a bit after pinning before continuing with the next
		// skylink. We'll try to roughly estimate how long we need to sleep
		// before the skylink is uploaded to its full redundancy but if we fail
		// we'll just sleep for a predefined interval.
		sleep, err := s.calculateSleep(skylink)
		if err != nil {
			s.staticLogger.Warn(errors.AddContext(err, "failed to get metadata for skylink"))
			sleep = SleepBetweenPins
		}
		select {
		case <-time.After(sleep):
		case <-s.staticTG.StopChan():
			return
		}
	}
}

// findAndPinOneUnderpinnedSkylink scans the database for one skylinks which is
// either locked by the current server or underpinned. If it finds such a
// skylink, it pins it to the local skyd. The method returns true until it finds
// no further skylinks to process.
func (s *Scanner) findAndPinOneUnderpinnedSkylink() (skylink string, continueScanning bool) {
	sl, err := s.staticDB.FindAndLockUnderpinned(context.TODO(), s.staticServerName, s.staticMinPinners)
	if errors.Contains(err, database.ErrSkylinkNotExist) {
		// No more underpinned skylinks pinnable by this server.
		return "", false
	}
	if err != nil {
		s.staticLogger.Warn(errors.AddContext(err, "failed to fetch underpinned skylink"))
		return "", true
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
		return "", true
	}
	s.staticLogger.Debugf("Successfully pinned %s", sl)
	return sl.String(), true
}

// calculateSleep calculates how long we should sleep after pinning the given
// skylink in order to give the renter time to fully upload it before we pin
// another one. It returns a ballpark value.
//
// This method makes some assumptions for simplicity:
// * assumes lazy pinning, meaning that none of the fanout is uploaded
// * all skyfiles are assumed to be large files (base sector + fanout) and the
//	metadata is assumed to fill up the base sector (to err on the safe side)
func (s *Scanner) calculateSleep(skylink string) (time.Duration, error) {
	meta, err := s.staticSkydClient.Metadata(skylink)
	if err != nil {
		return 0, err
	}
	numChunks := math.Ceil(float64(meta.Length) / chunkSize)
	// remainingUpload is the amount of data we expect to need to upload until
	// the skyfile reaches full redundancy.
	remainingUpload := numChunks*chunkSize*fanoutRedundancy + (baseSectorRedundancy-1)*sectorSize
	secondsRemaining := remainingUpload / assumedUploadSpeedInBytes
	return time.Duration(secondsRemaining) * time.Second, nil
}
