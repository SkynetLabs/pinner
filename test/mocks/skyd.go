package mocks

import "sync"

type (
	// SkydClientMock is a mock of skyd.Client
	SkydClientMock struct {
		pinnedSkylinks      []string
		pinError            error
		unpinError          error
		pinnedSkylinksError error

		mu sync.Mutex
	}
)

// IsPinning checks whether skyd is pinning the given skylink.
func (c *SkydClientMock) IsPinning(sl string) bool {
	for _, s := range c.pinnedSkylinks {
		if s == sl {
			return true
		}
	}
	return false
}

// Pin mocks a pin action and responds with a predefined error.
// If the predefined error is nil, it adds the given skylink to the list of
// skylinks pinned in the mock.
func (c *SkydClientMock) Pin(skylink string) error {
	if c.pinError == nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.pinnedSkylinks = append(c.pinnedSkylinks, skylink)
	}
	return c.pinError
}

// PinnedSkylinks is a mock.
func (c *SkydClientMock) PinnedSkylinks() (skylinks []string, err error) {
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
		for i, s := range c.pinnedSkylinks {
			if s == skylink {
				c.pinnedSkylinks = append(c.pinnedSkylinks[:i], c.pinnedSkylinks[i+1:]...)
			}
		}
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
