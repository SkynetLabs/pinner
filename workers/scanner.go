package workers

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/threadgroup"
)

/**
TODO
 PHASE 1:
 - scan the DB once a day for underpinned files
 - when you find an underpinned file that's not currently locked
	- lock it
	- pin it locally and add the current server to its list
	- unlock it
 PHASE 2:
 - calculate server load by getting the total number and size of files pinned by each server
 - only pin underpinned files if the current server is in the lowest 20% of servers, otherwise exit before scanning further
*/

const (
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
		staticCfg    conf.Config
		staticDB     *database.DB
		staticLogger *logrus.Logger
		staticTG     *threadgroup.ThreadGroup
	}
)

// NewScanner creates a new Scanner instance.
func NewScanner(cfg conf.Config, db *database.DB, logger *logrus.Logger, tg *threadgroup.ThreadGroup) *Scanner {
	return &Scanner{
		staticCfg:    cfg,
		staticDB:     db,
		staticLogger: logger,
		staticTG:     tg,
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
			case <-s.staticTG.StopChan():
				return
			case <-time.After(sleepBetweenScans):
			}
		}
	}()

	return nil
}

// threadedScanAndPin defines the scanning operation of Scanner.
func (s *Scanner) threadedScanAndPin() {
	/*
		TODO
		 - Unpin the skylink from the local server and remove the server from the list.
		 - Keep the skylink in the DB with the unpinning flag up. This will ensure that
		 if the skylink is still pinned to any server and we sweep that sever and add
		 the skylink to the DB, it will be immediately scheduled for unpinning and it
		 will be removed from that server.
	*/

}
