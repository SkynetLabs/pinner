package workers

import (
	"testing"
	"time"

	"github.com/skynetlabs/pinner/skyd"
	"github.com/skynetlabs/pinner/test"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

// TestScanner_calculateSleep ensures that estimateTimeToFull returns what we expect.
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

	skydMock := skyd.NewSkydClientMock()
	scanner := Scanner{
		staticSkydClient: skydMock,
	}
	skylink := test.RandomSkylink()

	for tname, tt := range tests {
		// Prepare the mock.
		meta := skymodules.SkyfileMetadata{Length: tt.dataSize}
		skydMock.SetMetadata(skylink.String(), meta, nil)

		sleep := scanner.estimateTimeToFull(skylink)
		if sleep != tt.expectedSleep {
			t.Errorf("%s: expected %ds, got %ds", tname, tt.expectedSleep/time.Second, sleep/time.Second)
		}
	}
}
