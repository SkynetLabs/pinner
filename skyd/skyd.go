package skyd

import (
	"fmt"
	"strings"

	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/logger"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/node/api"
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
		RebuildCache() RebuildCacheResult
		// RenterDirRootGet is a direct proxy to the skyd client method with the
		// same name.
		RenterDirRootGet(siaPath skymodules.SiaPath) (rd api.RenterDirectory, err error)
		// Resolve resolves a V2 skylink to a V1 skylink. Returns an error if
		// the given skylink is not V2.
		Resolve(skylink string) (string, error)
		// Unpin instructs the local skyd to unpin the given skylink.
		Unpin(skylink string) error
	}

	// client allows us to call the local skyd instance.
	client struct {
		staticClient        *skydclient.Client
		staticLogger        logger.ExtFieldLogger
		staticSkylinksCache *PinnedSkylinksCache
	}
)

// NewClient creates a new skyd client.
func NewClient(host, port, password string, cache *PinnedSkylinksCache, logger logger.ExtFieldLogger) Client {
	opts := skydclient.Options{
		Address:       fmt.Sprintf("%s:%s", host, port),
		Password:      password,
		UserAgent:     "Sia-Agent",
		CheckRedirect: nil,
	}
	return &client{
		staticClient:        skydclient.New(opts),
		staticLogger:        logger,
		staticSkylinksCache: cache,
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
	c.staticLogger.Trace("Entering FileHealth")
	defer c.staticLogger.Trace("Exiting  FileHealth")
	rf, err := c.staticClient.RenterFileRootGet(sp)
	if err != nil {
		return 0, err
	}
	return rf.File.Health, nil
}

// Metadata returns the metadata of the skylink
func (c *client) Metadata(skylink string) (skymodules.SkyfileMetadata, error) {
	c.staticLogger.Trace("Entering Metadata")
	defer c.staticLogger.Trace("Exiting  Metadata")
	_, meta, err := c.staticClient.SkynetMetadataGet(skylink)
	if err != nil {
		return skymodules.SkyfileMetadata{}, err
	}
	return meta, nil
}

// Pin instructs the local skyd to pin the given skylink.
func (c *client) Pin(skylink string) (skymodules.SiaPath, error) {
	c.staticLogger.Tracef("Entering Pin. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting  Pin. Skylink: '%s'", skylink)
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

// RebuildCache rebuilds the cache of skylinks pinned by the local skyd. The
// rebuilding happens in a goroutine, allowing the method to return a channel
// on which the caller can either wait or select. The caller can check whether
// the rebuild was successful by calling Error().
func (c *client) RebuildCache() RebuildCacheResult {
	c.staticLogger.Trace("Entering RebuildCache")
	defer c.staticLogger.Trace("Exiting  RebuildCache")
	return c.staticSkylinksCache.Rebuild(c)
}

// RenterDirRootGet is a direct proxy to skyd client's method.
func (c *client) RenterDirRootGet(siaPath skymodules.SiaPath) (rd api.RenterDirectory, err error) {
	return c.staticClient.RenterDirRootGet(siaPath)
}

// Resolve resolves a V2 skylink to a V1 skylink. Returns an error if the given
// skylink is not V2.
func (c *client) Resolve(skylink string) (string, error) {
	c.staticLogger.Tracef("Entering Resolve. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting  Resolve. Skylink: '%s'", skylink)
	return c.staticClient.ResolveSkylinkV2(skylink)
}

// Unpin instructs the local skyd to unpin the given skylink.
func (c *client) Unpin(skylink string) error {
	c.staticLogger.Tracef("Entering Unpin. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting  Unpin. Skylink: '%s'", skylink)
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
	defer c.staticLogger.Tracef("Exiting  isPinned. Skylink: '%s'", skylink)
	return c.staticSkylinksCache.Contains(skylink), nil
}
