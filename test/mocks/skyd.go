package mocks

import "sync"

type (
	// SkydClientMock is a mock of skyd.Client
	SkydClientMock struct {
		pinnedSkylinks      map[string]interface{}
		pinError            error
		unpinError          error
		pinnedSkylinksError error

		mu sync.Mutex
	}
)

// IsPinning checks whether skyd is pinning the given skylink.
func (c *SkydClientMock) IsPinning(skylink string) bool {
	_, exists := c.pinnedSkylinks[skylink]
	return exists
}

// Pin mocks a pin action and responds with a predefined error.
// If the predefined error is nil, it adds the given skylink to the list of
// skylinks pinned in the mock.
func (c *SkydClientMock) Pin(skylink string) error {
	if c.pinError == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.pinnedSkylinks[skylink] = struct{}{}
	}
	return c.pinError
}

// PinnedSkylinks is a mock.
func (c *SkydClientMock) PinnedSkylinks() (skylinks map[string]interface{}, err error) {
	if c.pinnedSkylinksError == nil {
		return c.pinnedSkylinks, nil
	}
	return nil, c.pinnedSkylinksError
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

// SetPinError set the pin error
func (c *SkydClientMock) SetPinError(e error) {
	c.pinError = e
}

// SetPinnedSkylinksError set the pinnedSkylinks error
func (c *SkydClientMock) SetPinnedSkylinksError(e error) {
	c.pinnedSkylinksError = e
}

// SetUnpinError set the unpin error
func (c *SkydClientMock) SetUnpinError(e error) {
	c.unpinError = e
}
