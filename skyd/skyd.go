package skyd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
	skydclient "gitlab.com/SkynetLabs/skyd/node/api/client"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"gitlab.com/SkynetLabs/skyd/skymodules/renter"
)

var (
	// ErrSkylinkAlreadyPinned is returned when the skylink we're trying to pin
	// is already pinned.
	ErrSkylinkAlreadyPinned = errors.New("skylink already pinned")
	// skylinksCache is a local cache of the list of skylinks pinned by the
	// local skyd
	skylinksCache pinnedSkylinksCache
	// skylinksCacheDuration defines the duration of skylinksCache
	skylinksCacheDuration = time.Hour
)

type (
	// Client describes the interface exposed by client.
	Client interface {
		FileHealth(sp skymodules.SiaPath) (float64, error)
		Metadata(skylink string) (skymodules.SkyfileMetadata, error)
		Pin(skylink string) (skymodules.SiaPath, error)
		PinnedSkylinks() (skylinks map[string]interface{}, err error)
		Resolve(skylink string) (string, error)
		Unpin(skylink string) error
	}

	// client allows us to call the local skyd instance.
	client struct {
		staticClient *skydclient.Client
		staticLogger *logrus.Logger
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
func NewClient(host, port, password string, logger *logrus.Logger) Client {
	opts := skydclient.Options{
		Address:       fmt.Sprintf("%s:%s", host, port),
		Password:      password,
		UserAgent:     "Sia-Agent",
		CheckRedirect: nil,
	}
	return &client{
		staticClient: skydclient.New(opts),
		staticLogger: logger,
	}
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
		c.managedUpdateCachedStatus(skylink, true)
	}
	return sp, err
}

// PinnedSkylinks returns the list of skylinks pinned by the local skyd.
func (c *client) PinnedSkylinks() (map[string]interface{}, error) {
	c.staticLogger.Trace("Entering PinnedSkylinks.")
	defer c.staticLogger.Trace("Exiting PinnedSkylinks.")
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
		// Skip the first element because that's current directory.
		for i := 1; i < len(rd.Directories); i++ {
			dirsToWalk = append(dirsToWalk, rd.Directories[i].SiaPath)
		}
	}
	// Update the cache.
	skylinksCache.skylinks = sls
	skylinksCache.expiration = time.Now().UTC().Add(skylinksCacheDuration)
	return sls, nil
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
		c.managedUpdateCachedStatus(skylink, false)
	}
	return err
}

// isPinned checks the list of skylinks pinned by the local skyd for the given
// skylink and returns true if it finds it.
func (c *client) isPinned(skylink string) (bool, error) {
	c.staticLogger.Tracef("Entering isPinned. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting isPinned. Skylink: '%s'", skylink)
	sls, err := c.PinnedSkylinks()
	if err != nil {
		return false, err
	}
	_, exists := sls[skylink]
	return exists, nil
}

// managedUpdateCachedStatus updates the cached status of the skylink - pinned
// or not.
func (c *client) managedUpdateCachedStatus(skylink string, pinned bool) {
	c.staticLogger.Tracef("Entering managedUpdateCachedStatus. Skylink: '%s'", skylink)
	defer c.staticLogger.Tracef("Exiting managedUpdateCachedStatus. Skylink: '%s'", skylink)
	skylinksCache.mu.Lock()
	defer skylinksCache.mu.Unlock()
	if pinned {
		skylinksCache.skylinks[skylink] = struct{}{}
	} else {
		delete(skylinksCache.skylinks, skylink)
	}
}
