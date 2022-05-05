package skyd

import (
	"fmt"
	"sync"
	"time"

	"gitlab.com/NebulousLabs/errors"
	skydclient "gitlab.com/SkynetLabs/skyd/node/api/client"
	"gitlab.com/SkynetLabs/skyd/skymodules"
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
		Skylinks   map[string]interface{}
		Expiration time.Time
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
	return c.staticClient.SkynetSkylinkPinPost(skylink, spp)
}

// PinnedSkylinks returns the list of skylinks pinned by the local skyd.
func (c *client) PinnedSkylinks() (map[string]interface{}, error) {
	skylinksCache.mu.Lock()
	defer skylinksCache.mu.Unlock()
	// Check whether the cache is still valid and return it if so.
	if skylinksCache.Expiration.After(time.Now().UTC()) {
		return skylinksCache.Skylinks, nil
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
	skylinksCache.Skylinks = sls
	skylinksCache.Expiration = time.Now().UTC().Add(skylinksCacheDuration)
	return sls, nil
}

// Unpin instructs the local skyd to unpin the given skylink.
func (c *client) Unpin(skylink string) error {
	pinned, err := c.isPinned(skylink)
	if err != nil {
		return err
	}
	if !pinned {
		// The skylink is not locally pinned, nothing to do.
		return nil
	}
	return c.staticClient.SkynetSkylinkUnpinPost(skylink)
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
