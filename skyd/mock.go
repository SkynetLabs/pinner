package skyd

import (
	"sync"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/node/api"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

type (
	// ClientMock is a mock of skyd.Client
	ClientMock struct {
		filesystemMock map[skymodules.SiaPath]rdReturnType
		metadata       map[string]skymodules.SkyfileMetadata
		metadataErrors map[string]error
		skylinks       map[string]struct{}
		pinError       error
		unpinError     error

		mu sync.Mutex
	}
	// rdReturnType describes the return values of RenterDirRootGet and allows
	// us to build a directory structure representation in NodeSkydClientMock.
	rdReturnType struct {
		RD  api.RenterDirectory
		Err error
	}
)

// NewSkydClientMock returns an initialised copy of ClientMock
func NewSkydClientMock() *ClientMock {
	return &ClientMock{
		filesystemMock: make(map[skymodules.SiaPath]rdReturnType),
		metadata:       make(map[string]skymodules.SkyfileMetadata),
		metadataErrors: make(map[string]error),
		skylinks:       make(map[string]struct{}),
	}
}

// DiffPinnedSkylinks is a carbon copy of PinnedSkylinksCache's version of the
// method.
func (c *ClientMock) DiffPinnedSkylinks(skylinks []string) (unknown []string, missing []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	removedMap := make(map[string]struct{}, len(c.skylinks))
	for sl := range c.skylinks {
		removedMap[sl] = struct{}{}
	}
	for _, sl := range skylinks {
		// Remove this skylink from the removedMap, because it has not been
		// removed.
		delete(removedMap, sl)
		// If it's not in the cache - add it to the added list.
		_, exists := c.skylinks[sl]
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

// FileHealth returns the health of the given skylink.
func (c *ClientMock) FileHealth(_ skymodules.SiaPath) (float64, error) {
	return 0, nil
}

// IsPinning checks whether skyd is pinning the given skylink.
func (c *ClientMock) IsPinning(skylink string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, exists := c.skylinks[skylink]
	return exists
}

// Metadata returns the metadata of the skylink or the pre-set error.
func (c *ClientMock) Metadata(skylink string) (skymodules.SkyfileMetadata, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.metadataErrors[skylink] != nil {
		return skymodules.SkyfileMetadata{}, c.metadataErrors[skylink]
	}
	return c.metadata[skylink], nil
}

// Pin mocks a pin action and responds with a predefined error.
// If the predefined error is nil, it adds the given skylink to the list of
// skylinks pinned in the mock.
func (c *ClientMock) Pin(skylink string) (skymodules.SiaPath, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pinError == nil {
		c.skylinks[skylink] = struct{}{}
	}
	sp := skymodules.SiaPath{
		Path: skylink,
	}
	return sp, c.pinError
}

// RebuildCache is a noop mock that takes at least 100ms.
func (c *ClientMock) RebuildCache() RebuildCacheResult {
	closedCh := make(chan struct{})
	close(closedCh)
	// Do some work. There are tests which rely on this value to be above 50ms.
	time.Sleep(100 * time.Millisecond)
	return RebuildCacheResult{
		errAvail:  closedCh,
		ErrAvail:  closedCh,
		ExternErr: nil,
	}
}

// RenterDirRootGet is a functional mock.
func (c *ClientMock) RenterDirRootGet(siaPath skymodules.SiaPath) (rd api.RenterDirectory, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, exists := c.filesystemMock[siaPath]
	if !exists {
		return api.RenterDirectory{}, errors.New("siapath does not exist")
	}
	return r.RD, r.Err
}

// SetMapping allows us to set the state of the filesystem mock.
func (c *ClientMock) SetMapping(siaPath skymodules.SiaPath, rdrt rdReturnType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.filesystemMock[siaPath] = rdrt
}

// Resolve is a noop mock.
func (c *ClientMock) Resolve(skylink string) (string, error) {
	return skylink, nil
}

// Unpin mocks an unpin action and responds with a predefined error.
// If the error is nil, Unpin removes the skylink from the list of pinned
// skylinks.
func (c *ClientMock) Unpin(skylink string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unpinError == nil {
		delete(c.skylinks, skylink)
	}
	return c.unpinError
}

// SetMetadata sets the metadata or error returned when fetching metadata for a
// given skylink. If both are provided the error takes precedence.
func (c *ClientMock) SetMetadata(skylink string, meta skymodules.SkyfileMetadata, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.metadata[skylink] = meta
	c.metadataErrors[skylink] = err
}

// SetPinError sets the pin error
func (c *ClientMock) SetPinError(e error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pinError = e
}

// SetUnpinError sets the unpin error
func (c *ClientMock) SetUnpinError(e error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unpinError = e
}

// MockFilesystem returns an initialised NodeSkydClientMock and a list of all
// skylinks contained in it.
//
// The mocked structure is the following:
//
// SkynetFolder/ (three dirs, one file)
//    dirA/ (two files, one skylink each)
//       fileA1 (A1_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg)
//       fileA2 (A2_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg)
//    dirB/ (one file, one dir)
//       dirC/ (one file, two skylinks)
//          fileC (C1_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg, C2_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg)
//       fileB (B__uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg)
//    dirD/ (empty)
//    file (___uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg)
func (c *ClientMock) MockFilesystem() []string {
	slR0 := "___uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"
	slA1 := "A1_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"
	slA2 := "A2_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"
	slC0 := "C1_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"
	slC1 := "C2_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"
	slB0 := "B__uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"

	dirAsp := skymodules.SiaPath{Path: "dirA"}
	dirBsp := skymodules.SiaPath{Path: "dirB"}
	dirCsp := skymodules.SiaPath{Path: "dirC"}
	dirDsp := skymodules.SiaPath{Path: "dirD"}

	root := skymodules.DirectoryInfo{SiaPath: skymodules.SkynetFolder}
	dirA := skymodules.DirectoryInfo{SiaPath: dirAsp}
	dirB := skymodules.DirectoryInfo{SiaPath: dirBsp}
	dirC := skymodules.DirectoryInfo{SiaPath: dirCsp}
	dirD := skymodules.DirectoryInfo{SiaPath: dirDsp}

	fileA1 := skymodules.FileInfo{Skylinks: []string{slA1}}
	fileA2 := skymodules.FileInfo{Skylinks: []string{slA2}}
	fileC0 := skymodules.FileInfo{Skylinks: []string{slC0, slC1}}
	fileB0 := skymodules.FileInfo{Skylinks: []string{slB0}}
	fileR0 := skymodules.FileInfo{Skylinks: []string{slR0}}

	// Set root.
	rdrt := rdReturnType{
		RD: api.RenterDirectory{
			Directories: []skymodules.DirectoryInfo{root, dirA, dirB, dirD},
			Files:       []skymodules.FileInfo{fileR0},
		},
		Err: nil,
	}
	c.SetMapping(skymodules.SkynetFolder, rdrt)
	// Set dirA.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: []skymodules.DirectoryInfo{dirA},
			Files:       []skymodules.FileInfo{fileA1, fileA2},
		},
		Err: nil,
	}
	c.SetMapping(dirAsp, rdrt)
	// Set dirB.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: []skymodules.DirectoryInfo{dirB, dirC},
			Files:       []skymodules.FileInfo{fileB0},
		},
		Err: nil,
	}
	c.SetMapping(dirBsp, rdrt)
	// Set dirC.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: []skymodules.DirectoryInfo{dirC},
			Files:       []skymodules.FileInfo{fileC0},
		},
		Err: nil,
	}
	c.SetMapping(dirCsp, rdrt)
	// Set dirD.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: nil,
			Files:       nil,
		},
		Err: nil,
	}
	c.SetMapping(dirDsp, rdrt)

	return []string{slR0, slA1, slA2, slC0, slC1, slB0}
}
