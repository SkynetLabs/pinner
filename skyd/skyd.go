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
	// skylinksCacheDuration defines the duration of skylinksCache
	skylinksCacheDuration = time.Hour
)

type (
	// Client describes the interface exposed by client.
	Client interface {
		Metadata(skylink string) (skymodules.SkyfileMetadata, error)
		Pin(skylink string) error
		PinnedSkylinks() (skylinks map[string]interface{}, err error)
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

// Metadata returns the metadata of the skylink
func (c *client) Metadata(skylink string) (skymodules.SkyfileMetadata, error) {
	_, meta, err := c.staticClient.SkynetMetadataGet(skylink)
	if err != nil {
		return skymodules.SkyfileMetadata{}, err
	}
	return meta, nil
}

// Pin instructs the local skyd to pin the given skylink.
func (c *client) Pin(skylink string) error {
	pinned, err := c.isPinned(skylink)
	if err != nil {
		return err
	}
	if pinned {
		// The skylink is already locally pinned, nothing to do.
		return nil
	}
	spp := skymodules.SkyfilePinParameters{
		SiaPath: skymodules.RandomSiaPath(),
	}
	err = c.staticClient.SkynetSkylinkPinPost(skylink, spp)
	if err != nil {
		c.updateCachedStatus(skylink, false)
	}
	return err
}

// PinnedSkylinks returns the list of skylinks pinned by the local skyd.
func (c *client) PinnedSkylinks() (map[string]interface{}, error) {
	skylinksCache.mu.Lock()
	defer skylinksCache.mu.Unlock()
	// Check whether the cache is still valid and return it if so.
	if skylinksCache.expiration.After(time.Now().UTC()) {
		return skylinksCache.skylinks, nil
	}
	// The cache is not valid, fetch the data from skyd.
	sls := make(map[string]interface{})
	dirsToWalk := []skymodules.SiaPath{skymodules.SkynetFolder}
	for len(dirsToWalk) > 0 {
		// Pop the first dir and walk it.
		dir := dirsToWalk[0]
		dirsToWalk = dirsToWalk[1:]

		rd, err := c.staticClient.RenterDirRootGet(dir)
		if err != nil {
			return nil, errors.AddContext(err, "failed to fetch skynet directories from skyd")
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
	skylinksCache.skylinks = sls
	skylinksCache.expiration = time.Now().UTC().Add(skylinksCacheDuration)
	return sls, nil
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
