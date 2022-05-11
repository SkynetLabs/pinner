package skyd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gitlab.com/NebulousLabs/errors"
	skydclient "gitlab.com/SkynetLabs/skyd/node/api/client"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"gitlab.com/SkynetLabs/skyd/skymodules/renter"
)

var (
	// skylinksCache is a local cache of the list of skylinks pinned by the
	// local skyd
	skylinksCache pinnedSkylinksCache
	// skylinksCacheTTL defines the duration of skylinksCache
	skylinksCacheTTL = time.Hour
)

type (
	// Client describes the interface exposed by client.
	Client interface {
		FileHealth(sp skymodules.SiaPath) (float64, error)
		Metadata(skylink string) (skymodules.SkyfileMetadata, error)
		Pin(skylink string) (skymodules.SiaPath, error)
		PinnedSkylinks() (skylinks map[string]interface{}, err error)
		RebuildCache() error
		Resolve(skylink string) (string, error)
		Unpin(skylink string) error
	}

	// client allows us to call the local skyd instance.
	client struct {
		staticClient *skydclient.Client
	}
	// renterDirCache is a simple cache of the renter's directory information,
	// so we don't need to fetch that for each skylink we potentially want to
	// pin/unpin.
	pinnedSkylinksCache struct {
		expiration time.Time
		skylinks   map[string]interface{}
		mu         sync.Mutex
	}
)

// NewClient creates a new skyd client.
func NewClient(host, port, password string) Client {
	opts := skydclient.Options{
		Address:       fmt.Sprintf("%s:%s", host, port),
		Password:      password,
		UserAgent:     "Sia-Agent",
		CheckRedirect: nil,
	}
	return &client{
		staticClient: skydclient.New(opts),
	}
}

// FileHealth returns the health of the given sia file.
// Perfect health is 0.
func (c *client) FileHealth(sp skymodules.SiaPath) (float64, error) {
	rf, err := c.staticClient.RenterFileRootGet(sp)
	if err != nil {
		return 0, err
	}
	return rf.File.Health, nil
}

// Metadata returns the metadata of the skylink
func (c *client) Metadata(skylink string) (skymodules.SkyfileMetadata, error) {
	_, meta, err := c.staticClient.SkynetMetadataGet(skylink)
	if err != nil {
		return skymodules.SkyfileMetadata{}, err
	}
	return meta, nil
}

// Pin instructs the local skyd to pin the given skylink.
func (c *client) Pin(skylink string) (skymodules.SiaPath, error) {
	pinned, err := c.isPinned(skylink)
	if err != nil {
		return skymodules.SiaPath{}, err
	}
	if pinned {
		// The skylink is already locally pinned, nothing to do.
		return skymodules.SiaPath{}, nil
	}
	sp, err := c.staticClient.SkynetSkylinkPinLazyPost(skylink)
	if err != nil {
		c.updateCachedStatus(skylink, false)
	}
	return sp, err
}

// PinnedSkylinks returns the list of skylinks pinned by the local skyd.
func (c *client) PinnedSkylinks() (map[string]interface{}, error) {
	skylinksCache.mu.Lock()
	defer skylinksCache.mu.Unlock()
	return skylinksCache.skylinks, nil
}

// RebuildCache rebuilds the cache of skylinks pinned by the local skyd.
func (c *client) RebuildCache() error {
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
		for _, d := range rd.Directories {
			dirsToWalk = append(dirsToWalk, d.SiaPath)
		}
	}
	// Update the cache.
	skylinksCache.mu.Lock()
	defer skylinksCache.mu.Unlock()
	skylinksCache.skylinks = sls
	skylinksCache.expiration = time.Now().UTC().Add(skylinksCacheTTL)
	return nil
}

// Resolve resolves a V2 skylink to a V1 skylink. Returns an error if the given
// skylink is not V2.
func (c *client) Resolve(skylink string) (string, error) {
	return c.staticClient.ResolveSkylinkV2(skylink)
}

// Unpin instructs the local skyd to unpin the given skylink.
func (c *client) Unpin(skylink string) error {
	err := c.staticClient.SkynetSkylinkUnpinPost(skylink)
	// Update the cached status of the skylink if there is no error or the error
	// indicates that the skylink is blocked.
	if err != nil || strings.Contains(err.Error(), renter.ErrSkylinkBlocked.Error()) {
		c.updateCachedStatus(skylink, false)
	}
	return err
}

// isPinned checks the list of skylinks pinned by the local skyd for the given
// skylink and returns true if it finds it.
func (c *client) isPinned(skylink string) (bool, error) {
	sls, err := c.PinnedSkylinks()
	if err != nil {
		return false, err
	}
	_, exists := sls[skylink]
	return exists, nil
}

// updateCachedStatus updates the cached status of the skylink - pinned or not.
func (c *client) updateCachedStatus(skylink string, pinned bool) {
	skylinksCache.mu.Lock()
	defer skylinksCache.mu.Unlock()
	if pinned {
		skylinksCache.skylinks[skylink] = struct{}{}
	} else {
		delete(skylinksCache.skylinks, skylink)
	}
}
