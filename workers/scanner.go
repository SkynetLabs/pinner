package workers

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/skyd"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/threadgroup"
)

/**
TODO
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

const (
	// sleepBetweenPins defines how long we'll sleep between pinning files.
	// We want to add this sleep in order to prevent a single server from
	// grabbing all underpinned files and overloading itself. We also want to
	// allow for some time for the newly pinned files to reach full redundancy
	// before we pin more files.
	sleepBetweenPins = 10 * time.Second
	// sleepBetweenScans defines how often we'll scan the DB for underpinned
	// skylinks.
	sleepBetweenScans = 24 * time.Hour
)

type (
	// Scanner is a background worker that periodically scans the database for
	// underpinned skylinks. Once an underpinned skylink is found (and it's not
	// being pinned by the local server already), Scanner pins it to the local
	// skyd.
	Scanner struct {
		staticCfg        conf.Config
		staticDB         *database.DB
		staticLogger     *logrus.Logger
		staticSkydClient *skyd.Client
		staticTG         *threadgroup.ThreadGroup
	}
)

// NewScanner creates a new Scanner instance.
func NewScanner(cfg conf.Config, db *database.DB, logger *logrus.Logger, skydClient *skyd.Client, tg *threadgroup.ThreadGroup) *Scanner {
	return &Scanner{
		staticCfg:        cfg,
		staticDB:         db,
		staticLogger:     logger,
		staticSkydClient: skydClient,
		staticTG:         tg,
	}
}

// Start launches the background worker thread that scans the DB for underpinned
// skylinks.
func (s *Scanner) Start() error {
	err := s.staticTG.Add()
	if err != nil {
		return err
	}

	go func() {
		defer s.staticTG.Done()
		for {
			s.threadedScanAndPin()
			select {
			case <-time.After(sleepBetweenScans):
			case <-s.staticTG.StopChan():
				return
			}
		}
	}()

	return nil
}

// threadedScanAndPin defines the scanning operation of Scanner.
func (s *Scanner) threadedScanAndPin() {
	for {
		// This function allows us to unlock the skylink in a defer.
		func() {
			sl, err := s.staticDB.SkylinkFetchAndLockUnderpinned(context.TODO(), s.staticCfg.ServerName, s.staticCfg.MinNumberOfPinners)
			if errors.Contains(err, database.ErrSkylinkNoExist) {
				// No more underpinned skylinks pinnable by this server.
				return
			}
			if err != nil {
				s.staticLogger.Warn(errors.AddContext(err, "failed to fetch underpinned skylink"))
				return
			}
			defer func() {
				err = s.staticDB.SkylinkUnlock(context.TODO(), sl, s.staticCfg.ServerName)
				if err != nil {
					s.staticLogger.Debug(errors.AddContext(err, "failed to unlock skylink after trying to pin it"))
				}
			}()
			err = s.staticSkydClient.Pin(sl)
			if err != nil {
				s.staticLogger.Warn(errors.AddContext(err, "failed to pin skylink"))
				return
			}
			s.staticLogger.Debugf("Successfully pinned %s", sl)
		}()

		// Sleep for a bit before continuing with the next skylink.
		select {
		case <-time.After(sleepBetweenPins):
		case <-s.staticTG.StopChan():
			return
		}
	}
}
