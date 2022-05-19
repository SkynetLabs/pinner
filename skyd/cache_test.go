package skyd

import (
	"testing"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/node/api"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

type (
	// SkydClientMock implements OwnSkydClient.
	SkydClientMock struct {
		presets map[skymodules.SiaPath]rdReturnType
	}
	// rdReturnType describes the return values of RenterDirRootGet and allows
	// us to build a directory structure representation in SkydClientMock.
	rdReturnType struct {
		RD  api.RenterDirectory
		Err error
	}
)

// NewSkydClientMock returns a new mock of skydclient.Client which satisfies the
// OwnSkydClient interface.
func NewSkydClientMock() *SkydClientMock {
	return &SkydClientMock{
		make(map[skymodules.SiaPath]rdReturnType),
	}
}

// RenterDirRootGet is a functional mock.
func (scm *SkydClientMock) RenterDirRootGet(siaPath skymodules.SiaPath) (rd api.RenterDirectory, err error) {
	r, exists := scm.presets[siaPath]
	if !exists {
		return api.RenterDirectory{}, errors.New("siapath does not exist")
	}
	return r.RD, r.Err
}

// SetMapping allows us to set the internal state of the mock.
func (scm *SkydClientMock) SetMapping(siaPath skymodules.SiaPath, rdrt rdReturnType) {
	scm.presets[siaPath] = rdrt
}

// TestCacheBase covers the base functionality of PinnerSkylinksCache:
// * NewCache
// * Add
// * Contains
// * Diff
// * Remove
func TestCacheBase(t *testing.T) {
	t.Parallel()

	sl1 := "A_CuSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"
	sl2 := "B_CuSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"
	sl3 := "C_CuSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"

	c := NewCache()
	if c.Contains(sl1) {
		t.Fatal("Should not contain ", sl1)
	}
	c.Add(sl1)
	if !c.Contains(sl1) {
		t.Fatal("Should contain ", sl1)
	}
	c.Remove(sl1)
	if c.Contains(sl1) {
		t.Fatal("Should not contain ", sl1)
	}

	// Add sl1 and sl2 to the cache.
	c.Add(sl1)
	c.Add(sl2)
	// Diff a list of sl2 and sl3 against the cache.
	// Expect to get sl1 as missing and sl3 as unknown.
	u, m := c.Diff([]string{sl2, sl3})
	if len(m) != 1 || m[0] != sl1 {
		t.Fatalf("Expected to get '%s' as the single 'missing' result but got %v", sl3, m)
	}
	if len(u) != 1 || u[0] != sl3 {
		t.Fatalf("Expected to get '%s' as the single 'unknown' result but got %v", sl1, u)
	}
}

// TestCacheRebuild covers the Rebuild functionality of PinnerSkylinksCache.
func TestCacheRebuild(t *testing.T) {
	t.Parallel()

	sl := "XX_uSb3BpGxmSbRAg1xj5T8SdB4hiSFiEW2sEEzxt5MNkg"

	c := NewCache()
	// Add a skylink to the cache. Expect this to be gone after the rebuild.
	c.Add(sl)
	skyd, sls := mock()
	rr := c.Rebuild(skyd)
	// Wait for the rebuild to finish.
	<-rr.Ch
	if rr.ExternErr != nil {
		t.Fatal(rr.ExternErr)
	}
	// Ensure that all expected skylinks are in the cache now.
	for _, s := range sls {
		if !c.Contains(s) {
			t.Fatalf("Expected skylink '%s' to be in the cache.", s)
		}
	}
	// Ensure that the skylink we added before the rebuild is gone.
	if c.Contains(sl) {
		t.Fatalf("Expected skylink '%s' to not be present after the rebuild.", sl)
	}
}

// mock returns an initialised SkydClientMock and a list of all skylinks
// contained in it.
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
func mock() (*SkydClientMock, []string) {
	skyd := NewSkydClientMock()

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
	skyd.SetMapping(skymodules.SkynetFolder, rdrt)
	// Set dirA.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: []skymodules.DirectoryInfo{dirA},
			Files:       []skymodules.FileInfo{fileA1, fileA2},
		},
		Err: nil,
	}
	skyd.SetMapping(dirAsp, rdrt)
	// Set dirB.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: []skymodules.DirectoryInfo{dirB, dirC},
			Files:       []skymodules.FileInfo{fileB0},
		},
		Err: nil,
	}
	skyd.SetMapping(dirBsp, rdrt)
	// Set dirC.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: []skymodules.DirectoryInfo{dirC},
			Files:       []skymodules.FileInfo{fileC0},
		},
		Err: nil,
	}
	skyd.SetMapping(dirCsp, rdrt)
	// Set dirD.
	rdrt = rdReturnType{
		RD: api.RenterDirectory{
			Directories: nil,
			Files:       nil,
		},
		Err: nil,
	}
	skyd.SetMapping(dirDsp, rdrt)

	return skyd, []string{slR0, slA1, slA2, slC0, slC1, slB0}
}
