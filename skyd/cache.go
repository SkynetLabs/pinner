package skyd

import (
	"sync"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
	"gitlab.com/SkynetLabs/skyd/node/api"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

type (
	// PinnedSkylinksCache is a simple cache of the renter's directory
	// information, so we don't need to fetch that for each skylink we
	// potentially want to pin/unpin.
	PinnedSkylinksCache struct {
		result   *RebuildCacheResult
		skylinks map[string]struct{}
		mu       sync.Mutex
	}
	// RebuildCacheResult informs the caller on the status of a cache rebuild.
	// The error should not be read before the channel is closed.
	RebuildCacheResult struct {
		// errAvail indicates the status of the cache rebuild progress.
		// We expose this same channel as <-chan ErrAvail.
		errAvail chan struct{}
		// ErrAvail indicates the status of the cache rebuild progress -
		// if it's not closed then the rebuild is still in progress. We expose
		// it as a <-chan, so the receiver cannot close it.
		ErrAvail <-chan struct{}
		// ExternErr holds the error state of the cache rebuild process. It must
		// only be read after ErrAvail is closed.
		ExternErr error
	}
)

// NewCache returns a new cache instance.
func NewCache() *PinnedSkylinksCache {
	return &PinnedSkylinksCache{
		result:   nil,
		skylinks: make(map[string]struct{}),
		mu:       sync.Mutex{},
	}
}

// Add registers the given skylinks in the cache.
func (psc *PinnedSkylinksCache) Add(skylinks ...string) {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	for _, s := range skylinks {
		psc.skylinks[s] = struct{}{}
	}
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
	removedMap := make(map[string]struct{}, len(psc.skylinks))
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

// Rebuild rebuilds the cache of skylinks pinned by the local skyd. The
// rebuilding happens in a goroutine, allowing the method to return a channel
// on which the caller can either wait or select. The caller can check whether
// the rebuild was successful by calling Error().
func (psc *PinnedSkylinksCache) Rebuild(skydClient Client) RebuildCacheResult {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	if !psc.isRebuildInProgress() {
		psc.result = NewRebuildCacheResult()
		// Kick off the actual rebuild in a separate goroutine.
		go psc.threadedRebuild(skydClient)
	}
	return *psc.result
}

// Remove removes the given skylinks in the cache.
func (psc *PinnedSkylinksCache) Remove(skylinks ...string) {
	psc.mu.Lock()
	defer psc.mu.Unlock()
	for _, s := range skylinks {
		delete(psc.skylinks, s)
	}
}

// isRebuildInProgress returns true if a cache rebuild is in progress.
// Calling this method assumes that caller is holding a lock on the cache.
func (psc *PinnedSkylinksCache) isRebuildInProgress() bool {
	return psc.result != nil
}

// threadedRebuild performs the actual cache rebuild process. It reports any
// errors by setting the psc.err variable and it always closes the rebuildCh on
// exit.
func (psc *PinnedSkylinksCache) threadedRebuild(skydClient Client) {
	var err error
	// Ensure that we properly wrap up the rebuild process.
	defer func() {
		psc.mu.Lock()
		// Update the result.
		psc.result.ExternErr = err
		psc.result.close()
		// Mark the rebuild as done.
		psc.result = nil
		psc.mu.Unlock()
	}()

	// Walk the entire Skynet folder and scan all files we find for skylinks.
	dirsToWalk := []skymodules.SiaPath{skymodules.SkynetFolder}
	sls := make(map[string]struct{})
	var rd api.RenterDirectory
	for len(dirsToWalk) > 0 {
		// Pop the first dir and walk it.
		dir := dirsToWalk[0]
		dirsToWalk = dirsToWalk[1:]
		rd, err = skydClient.RenterDirRootGet(dir)
		if err != nil {
			err = errors.AddContext(err, "failed to fetch skynet directories from skyd")
			return
		}
		for _, f := range rd.Files {
			for _, sl := range f.Skylinks {
				sls[sl] = struct{}{}
			}
		}
		// Grab all subdirs and queue them for walking.
		// Skip the first element because that's current directory.
		for i := 1; i < len(rd.Directories); i++ {
			dirsToWalk = append(dirsToWalk, rd.Directories[i].SiaPath)
		}
	}

	// Update the cache.
	psc.mu.Lock()
	psc.skylinks = sls
	psc.mu.Unlock()
}

// NewRebuildCacheResult returns a new RebuildCacheResult
func NewRebuildCacheResult() *RebuildCacheResult {
	ch := make(chan struct{})
	return &RebuildCacheResult{
		errAvail:  ch,
		ErrAvail:  ch,
		ExternErr: nil,
	}
}

// close ensures that we don't try to close the results channel more than once.
func (rr *RebuildCacheResult) close() {
	select {
	case <-rr.ErrAvail:
		build.Critical("double close on a results channel")
		return
	default:
	}
	close(rr.errAvail)
}
