package sweeper

import (
	"sync"
	"time"
)

type (
	// schedule defines how often, if at all, we sweep this server automatically.
	schedule struct {
		period   time.Duration
		cancelCh chan struct{}
		mu       sync.Mutex
	}
)

// Update schedules a new series of sweeps to be run, using the given Sweeper.
// If there are already scheduled sweeps, that schedule is cancelled (running
// sweeps are not interrupted) and a new schedule is established.
func (s *schedule) Update(period time.Duration, sweeper *Sweeper) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if isOpen(s.cancelCh) {
		close(s.cancelCh)
	}

	s.period = period
	s.cancelCh = make(chan struct{})

	go func() {
		t := time.NewTicker(s.period)
		for {
			select {
			case <-t.C:
				sweeper.Sweep()
			case <-s.cancelCh:
				return
			}
		}
	}()
}

// isOpen checks whether a channel is open (and not nil).
// The question the function answers is "Can I close this?"
func isOpen(ch chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case _, open := <-ch:
		return open
	default:
		return true
	}
}
