package workers

import (
	"testing"
	"time"

	"github.com/skynetlabs/pinner/test"
	"github.com/skynetlabs/pinner/test/mocks"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

// TestScanner_calculateSleep ensures that calculateSleep returns what we expect.
func TestScanner_calculateSleep(t *testing.T) {
	tests := map[string]struct {
		dataSize      uint64
		expectedSleep time.Duration
	}{
		"small file": {
			1 << 20, // 1 MB
			4 * time.Second,
		},
		"5 MB": {
			1 << 20 * 5, // 5 MB
			4 * time.Second,
		},
		"50 MB": {
			1 << 20 * 50, // 50 MB
			8 * time.Second,
		},
		"500 MB": {
			1 << 20 * 500, // 500 MB
			49 * time.Second,
		},
		"5 GB": {
			1 << 30 * 5, // 5 GB
			481 * time.Second,
		},
	}

	skydMock := mocks.NewSkydClientMock()
	scanner := Scanner{
		staticSkydClient: skydMock,
	}
	skylink := test.RandomSkylink().String()

	for tname, tt := range tests {
		// Prepare the mock.
		meta := skymodules.SkyfileMetadata{Length: tt.dataSize}
		skydMock.SetMetadata(skylink, meta, nil)

		sleep, err := scanner.calculateSleep(skylink)
		if err != nil {
			t.Fatalf("%s: failed with error %v", tname, err)
		}
		if sleep != tt.expectedSleep {
			t.Errorf("%s: expected %ds, got %ds", tname, tt.expectedSleep/time.Second, sleep/time.Second)
		}
	}
}
