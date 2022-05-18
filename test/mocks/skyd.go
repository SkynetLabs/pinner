package mocks

import (
	"sync"

	"github.com/skynetlabs/pinner/skyd"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

type (
	// SkydClientMock is a mock of skyd.Client
	SkydClientMock struct {
		pinnedSkylinks      map[string]struct{}
		pinError            error
		unpinError          error
		pinnedSkylinksError error
		metadata            map[string]skymodules.SkyfileMetadata
		metadataErrors      map[string]error

		mu sync.Mutex
	}
)

// NewSkydClientMock returns an initialised copy of SkydClientMock
func NewSkydClientMock() *SkydClientMock {
	return &SkydClientMock{
		pinnedSkylinks: make(map[string]struct{}),
		metadata:       make(map[string]skymodules.SkyfileMetadata),
		metadataErrors: make(map[string]error),
	}
}

// DiffPinnedSkylinks is a mock.
func (c *SkydClientMock) DiffPinnedSkylinks(_ []string) (unknown []string, missing []string) {
	return nil, nil
}

// FileHealth returns the health of the given skylink.
func (c *SkydClientMock) FileHealth(_ skymodules.SiaPath) (float64, error) {
	return 0, nil
}

// IsPinning checks whether skyd is pinning the given skylink.
func (c *SkydClientMock) IsPinning(skylink string) bool {
	_, exists := c.pinnedSkylinks[skylink]
	return exists
}

// Metadata returns the metadata of the skylink or the pre-set error.
func (c *SkydClientMock) Metadata(skylink string) (skymodules.SkyfileMetadata, error) {
	if c.metadataErrors[skylink] != nil {
		return skymodules.SkyfileMetadata{}, c.metadataErrors[skylink]
	}
	return c.metadata[skylink], nil
}

// Pin mocks a pin action and responds with a predefined error.
// If the predefined error is nil, it adds the given skylink to the list of
// skylinks pinned in the mock.
func (c *SkydClientMock) Pin(skylink string) (skymodules.SiaPath, error) {
	if c.pinError == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.pinnedSkylinks[skylink] = struct{}{}
	}
	sp := skymodules.SiaPath{
		Path: skylink,
	}
	return sp, c.pinError
}

// PinnedSkylinks is a mock.
func (c *SkydClientMock) PinnedSkylinks() (skylinks map[string]struct{}, err error) {
	if c.pinnedSkylinksError == nil {
		return c.pinnedSkylinks, nil
	}
	return nil, c.pinnedSkylinksError
}

// RebuildCache is a noop mock.
func (c *SkydClientMock) RebuildCache() skyd.RebuildCacheResult {
	closedCh := make(chan struct{})
	close(closedCh)
	return skyd.RebuildCacheResult{
		Ch:        closedCh,
		ExternErr: nil,
	}
}

// Resolve is a noop mock.
func (c *SkydClientMock) Resolve(skylink string) (string, error) {
	return skylink, nil
}

// Unpin mocks an unpin action and responds with a predefined error.
// If the error is nil, Unpin removes the skylink from the list of pinned
// skylinks.
func (c *SkydClientMock) Unpin(skylink string) error {
	if c.unpinError == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.pinnedSkylinks, skylink)
	}
	return c.unpinError
}

// SetMetadata sets the metadata or error returned when fetching metadata for a
// given skylink. If both are provided the error takes precedence.
func (c *SkydClientMock) SetMetadata(skylink string, meta skymodules.SkyfileMetadata, err error) {
	c.metadata[skylink] = meta
	c.metadataErrors[skylink] = err
}

// SetPinError sets the pin error
func (c *SkydClientMock) SetPinError(e error) {
	c.pinError = e
}

// SetPinnedSkylinksError sets the pinnedSkylinks error
func (c *SkydClientMock) SetPinnedSkylinksError(e error) {
	c.pinnedSkylinksError = e
}

// SetUnpinError sets the unpin error
func (c *SkydClientMock) SetUnpinError(e error) {
	c.unpinError = e
}
