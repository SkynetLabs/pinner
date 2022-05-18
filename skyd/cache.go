package skyd

import (
	"sync"

	"gitlab.com/NebulousLabs/errors"
)

var (
	// ErrNoRebuildInProgress is returned when we try to finish a rebuild but
	// none is in progress.
	ErrNoRebuildInProgress = errors.New("no rebuild in progress")
	// ErrRebuildInProgress is returned when we try to start a rebuild and one
	// is already in progress.
	ErrRebuildInProgress = errors.New("rebuild in progress")
)

type (
	// PinnedSkylinksCache is a simple cache of the renter's directory
	// information, so we don't need to fetch that for each skylink we
	// potentially want to pin/unpin.
	PinnedSkylinksCache struct {
		rebuildCh chan interface{}
		skylinks  map[string]struct{}
		mu        sync.Mutex
	}
)

// NewCache returns a new cache instance.
func NewCache() *PinnedSkylinksCache {
	closedCh := make(chan interface{})
	close(closedCh)
	return &PinnedSkylinksCache{
		rebuildCh: closedCh,
		skylinks:  nil,
		mu:        sync.Mutex{},
	}
}

// Add registers the given skylink in the cache.
func (psc *PinnedSkylinksCache) Add(skylink string) {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	if psc.skylinks == nil {
		psc.skylinks = make(map[string]struct{})
	}
	psc.skylinks[skylink] = struct{}{}
}

// Contains returns true when the given skylink is in the cache.
func (psc *PinnedSkylinksCache) Contains(skylink string) bool {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	_, exists := psc.skylinks[skylink]
	return exists
}

// Diff returns two lists of skylinks - the ones that are in the given list but
// are not in the cache (missing) and the ones that are in the cache but are not
// in the given list (removed).
func (psc *PinnedSkylinksCache) Diff(sls []string) (unknown []string, missing []string) {
	psc.mu.Lock()
	defer psc.mu.Unlock()

	removedMap := make(map[string]struct{})
	for sl := range psc.skylinks {
		removedMap[sl] = struct{}{}
	}
	for _, sl := range sls {
		// Remove this skylink from the removedMap, because it has not been
		// removed.
		delete(removedMap, sl)
		// If it's not in the cache - add it to the added list.
		_, exists := psc.skylinks[sl]
		if !exists {
			unknown = append(unknown, sl)
		}
	}
	// Transform the removed map into a list.
	for sl := range removedMap {
		missing = append(missing, sl)
	}
	return
}

// Remove registers the given skylink in the cache.
func (psc *PinnedSkylinksCache) Remove(skylink string) {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	if psc.skylinks == nil {
		psc.skylinks = make(map[string]struct{})
	}
	delete(psc.skylinks, skylink)
}

// blockingWaitForRebuild blocks until the current cache rebuild process ends.
func (psc *PinnedSkylinksCache) blockingWaitForRebuild() {
	<-psc.rebuildCh
	return
}

// managedIsRebuildInProgress returns true if a cache rebuild is in progress.
func (psc *PinnedSkylinksCache) managedIsRebuildInProgress() bool {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	select {
	case <-psc.rebuildCh:
		return false
	default:
		return true
	}
}

// managedReplaceCache replaces the entire cache content.
func (psc *PinnedSkylinksCache) managedReplaceCache(newCache map[string]struct{}) {
	psc.mu.Lock()
	psc.skylinks = newCache
	psc.mu.Unlock()
}

// managedSignalRebuildEnd marks the end of a cache rebuild.
func (psc *PinnedSkylinksCache) managedSignalRebuildEnd() error {
	if !psc.managedIsRebuildInProgress() {
		return ErrNoRebuildInProgress
	}
	psc.mu.Lock()
	close(psc.rebuildCh)
	psc.mu.Unlock()
	return nil
}

// managedSignalRebuildStart marks the start of a cache rebuild.
func (psc *PinnedSkylinksCache) managedSignalRebuildStart() error {
	if psc.managedIsRebuildInProgress() {
		return ErrRebuildInProgress
	}
	psc.mu.Lock()
	psc.rebuildCh = make(chan interface{})
	psc.mu.Unlock()
	return nil
}