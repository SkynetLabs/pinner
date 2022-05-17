package skyd

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
	skydclient "gitlab.com/SkynetLabs/skyd/node/api/client"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"gitlab.com/SkynetLabs/skyd/skymodules/renter"
)

var (
	// ErrSkylinkAlreadyPinned is returned when the skylink we're trying to pin
	// is already pinned.
	ErrSkylinkAlreadyPinned = errors.New("skylink already pinned")
)

type (
	// Client describes the interface exposed by client.
	Client interface {
		// DiffPinnedSkylinks returns two lists of skylinks - the ones that
		// belong to the given list but are not pinned by skyd (unknown) and the
		// ones that are pinned by skyd but are not on the list (missing).
		DiffPinnedSkylinks(skylinks []string) (unknown []string, missing []string)
		// FileHealth returns the health of the given sia file.
		// Perfect health is 0.
		FileHealth(sp skymodules.SiaPath) (float64, error)
		// Metadata returns the metadata of the skylink
		Metadata(skylink string) (skymodules.SkyfileMetadata, error)
		// Pin instructs the local skyd to pin the given skylink.
		Pin(skylink string) (skymodules.SiaPath, error)
		// RebuildCache rebuilds the cache of skylinks pinned by the local skyd.
		RebuildCache() error
		// Resolve resolves a V2 skylink to a V1 skylink. Returns an error if
		// the given skylink is not V2.
		Resolve(skylink string) (string, error)
		// Unpin instructs the local skyd to unpin the given skylink.
		Unpin(skylink string) error
	}

	// client allows us to call the local skyd instance.
	client struct {
		staticClient        *skydclient.Client
		staticLogger        *logrus.Logger
		staticSkylinksCache *pinnedSkylinksCache
	}
)

// NewClient creates a new skyd client.
func NewClient(host, port, password string, logger *logrus.Logger) Client {
	opts := skydclient.Options{
		Address:       fmt.Sprintf("%s:%s", host, port),
		Password:      password,
		UserAgent:     "Sia-Agent",
		CheckRedirect: nil,
	}
	return &client{
		staticClient:        skydclient.New(opts),
		staticLogger:        logger,
		staticSkylinksCache: skylinksCache,
	}
}

// DiffPinnedSkylinks returns two lists of skylinks - the ones that belong to
// the given list but are not pinned by skyd (unknown) and the ones that are
// pinned by skyd but are not on the list (missing).
func (c *client) DiffPinnedSkylinks(skylinks []string) (unknown []string, missing []string) {
	return c.staticSkylinksCache.Diff(skylinks)
}

// FileHealth returns the health of the given sia file.
// Perfect health is 0.
func (c *client) FileHealth(sp skymodules.SiaPath) (float64, error) {
	c.staticLogger.Trace("Entering FileHealth.")
	defer c.staticLogger.Trace("Exiting FileHealth.")
	rf, err := c.staticClient.RenterFileRootGet(sp)
	if err != nil {
		return 0, err
	}
	return rf.File.Health, nil
}

// Metadata returns the metadata of the skylink
func (c *client) Metadata(skylink string) (skymodules.SkyfileMetadata, error) {
	c.staticLogger.Trace("Entering Metadata.")
	defer c.staticLogger.Trace("Exiting Metadata.")
	_, meta, err := c.staticClient.SkynetMetadataGet(skylink)
	if err != nil {
		return skymodules.SkyfileMetadata{}, err
	}
	return meta, nil
}

// Pin instructs the local skyd to pin the given skylink.
func (c *client) Pin(skylink string) (skymodules.SiaPath, error) {
	c.staticLogger.Tracef("Entering Pin. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting Pin. Skylink: '%s'", skylink)
	_, err := database.SkylinkFromString(skylink)
	if err != nil {
		return skymodules.SiaPath{}, errors.Compose(err, database.ErrInvalidSkylink)
	}
	pinned, err := c.isPinned(skylink)
	if err != nil {
		return skymodules.SiaPath{}, err
	}
	if pinned {
		// The skylink is already locally pinned, nothing to do.
		return skymodules.SiaPath{}, ErrSkylinkAlreadyPinned
	}
	sp, err := c.staticClient.SkynetSkylinkPinLazyPost(skylink)
	if err == nil || errors.Contains(err, ErrSkylinkAlreadyPinned) {
		c.staticSkylinksCache.Add(skylink)
	}
	return sp, err
}

// RebuildCache rebuilds the cache of skylinks pinned by the local skyd.
func (c *client) RebuildCache() error {
	// Signal  cache rebuild start.
	err := c.staticSkylinksCache.managedSignalRebuildStart()
	if errors.Contains(err, ErrRebuildInProgress) {
		// A rebuild is already in progress. All we need to do is wait for it to
		// finish and return a success.
		c.staticSkylinksCache.blockingWaitForRebuild()
		return nil
	}

	// Rebuild the cache.
	sls := make(map[string]interface{})
	dirsToWalk := []skymodules.SiaPath{skymodules.SkynetFolder}
	for len(dirsToWalk) > 0 {
		// Pop the first dir and walk it.
		dir := dirsToWalk[0]
		dirsToWalk = dirsToWalk[1:]
		rd, err := c.staticClient.RenterDirRootGet(dir)
		if err != nil {
			return errors.AddContext(err, "failed to fetch skynet directories from skyd")
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
	c.staticSkylinksCache.managedReplaceCache(sls)

	// Signal a cache rebuild end.
	err = c.staticSkylinksCache.managedSignalRebuildEnd()
	if err != nil {
		// This should never happen, so we log a build.Critical. We don't need
		// to return an error to the caller, though, because the cache was
		// rebuilt and from their perspective everything is fine.
		build.Critical(errors.AddContext(err, "we started a rebuild but somebody else finished it for us"))
	}

	return nil
}

// Resolve resolves a V2 skylink to a V1 skylink. Returns an error if the given
// skylink is not V2.
func (c *client) Resolve(skylink string) (string, error) {
	c.staticLogger.Tracef("Entering Resolve. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting Resolve. Skylink: '%s'", skylink)
	return c.staticClient.ResolveSkylinkV2(skylink)
}

// Unpin instructs the local skyd to unpin the given skylink.
func (c *client) Unpin(skylink string) error {
	c.staticLogger.Tracef("Entering Unpin. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting Unpin. Skylink: '%s'", skylink)
	err := c.staticClient.SkynetSkylinkUnpinPost(skylink)
	// Update the cached status of the skylink if there is no error or the error
	// indicates that the skylink is blocked.
	if err != nil || strings.Contains(err.Error(), renter.ErrSkylinkBlocked.Error()) {
		c.staticSkylinksCache.Remove(skylink)
	}
	return err
}

// isPinned checks the list of skylinks pinned by the local skyd for the given
// skylink and returns true if it finds it.
func (c *client) isPinned(skylink string) (bool, error) {
	c.staticLogger.Tracef("Entering isPinned. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting isPinned. Skylink: '%s'", skylink)
	return c.staticSkylinksCache.Contains(skylink), nil
}
