package workers

import (
	"context"
	"testing"
	"time"

	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/skyd"
	"github.com/skynetlabs/pinner/test"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/build"
	"gitlab.com/SkynetLabs/skyd/skymodules"
)

const (
	// cyclesToWait establishes a common number of sleepBetweenScans cycles we
	// should wait until we consider that a file has been or hasn't been picked
	// by the scanner.
	cyclesToWait = 5
)

var (
	// maxSleepBetweenScans is the maximum time we might sleep between scans.
	maxSleepBetweenScans = time.Duration(float64(sleepBetweenScans) * (1 + sleepVariationFactor))
)

// TestScanner ensures that Scanner does its job.
func TestScanner(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ctx := context.Background()
	db, err := test.NewDatabase(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := test.LoadTestConfig()
	if err != nil {
		t.Fatal(err)
	}
	skydcm := skyd.NewSkydClientMock()
	scanner := NewScanner(db, test.NewDiscardLogger(), cfg.MinPinners, cfg.ServerName, cfg.SleepBetweenScans, skydcm)
	defer func() {
		if e := scanner.Close(); e != nil {
			t.Error(errors.AddContext(e, "failed to close threadgroup"))
		}
	}()
	err = scanner.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Add a skylink from the name of a different server.
	sl := test.RandomSkylink()
	otherServer := "other server"
	_, err = db.CreateSkylink(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}

	// Sleep for a while, giving a chance to the scanner to pick the skylink up.
	time.Sleep(cyclesToWait * scanner.SleepBetweenScans())
	// Make sure the skylink isn't pinned on the local (mock) skyd.
	if skydcm.IsPinning(sl.String()) {
		t.Fatal("We didn't expect skyd to be pinning this.")
	}
	// Remove the other server, making the file underpinned.
	err = db.RemoveServerFromSkylink(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the skylink should be picked up and pinned on the local skyd.
	err = build.Retry(cyclesToWait, scanner.SleepBetweenScans(), func() error {
		// Make sure the skylink is pinned on the local (mock) skyd.
		if !skydcm.IsPinning(sl.String()) {
			return errors.New("we expected skyd to be pinning this")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestScannerDryRun ensures that dry_run works as expected.
func TestScannerDryRun(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	// Don't run this test in parallel since we set "dry_run". mongo is shared
	// by the tests.

	ctx := context.Background()
	db, err := test.NewDatabase(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	// Set dry_run: true.
	err = db.SetConfigValue(ctx, conf.ConfDryRun, "true")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		err = db.SetConfigValue(ctx, conf.ConfDryRun, "false")
		if err != nil {
			t.Fatal(err)
		}
	}()

	cfg, err := test.LoadTestConfig()
	if err != nil {
		t.Fatal(err)
	}
	skydcm := skyd.NewSkydClientMock()
	scanner := NewScanner(db, test.NewDiscardLogger(), cfg.MinPinners, cfg.ServerName, cfg.SleepBetweenScans, skydcm)
	defer func() {
		if e := scanner.Close(); e != nil {
			t.Error(errors.AddContext(e, "failed to close threadgroup"))
		}
	}()
	err = scanner.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Trigger a pin event.
	//
	// Add a skylink from the name of a different server.
	sl := test.RandomSkylink()
	otherServer := "other server"
	_, err = db.CreateSkylink(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}
	// Sleep for a while, giving a chance to the scanner to pick the skylink up.
	time.Sleep(cyclesToWait * scanner.SleepBetweenScans())
	// Make sure the skylink isn't pinned on the local (mock) skyd.
	if skydcm.IsPinning(sl.String()) {
		t.Fatal("We didn't expect skyd to be pinning this.")
	}
	// Remove the other server, making the file underpinned.
	err = db.RemoveServerFromSkylink(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}

	// Wait - the skylink should not be picked up and pinned on the local skyd.
	time.Sleep(cyclesToWait * scanner.SleepBetweenScans())

	// Verify skyd doesn't have the pin.
	//
	// Make sure the skylink is not pinned on the local (mock) skyd.
	if skydcm.IsPinning(sl.String()) {
		t.Fatal("We did not expect skyd to be pinning this.")
	}

	// Turn off dry run.
	err = db.SetConfigValue(ctx, conf.ConfDryRun, "false")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the skylink should be picked up and pinned on the local skyd.
	err = build.Retry(2*cyclesToWait, scanner.SleepBetweenScans(), func() error {
		// Make sure the skylink is pinned on the local (mock) skyd.
		if !skydcm.IsPinning(sl.String()) {
			return errors.New("we expected skyd to be pinning this")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestScanner_calculateSleep ensures that estimateTimeToFull returns what we expect.
func TestScanner_calculateSleep(t *testing.T) {
	tests := map[string]struct {
		dataSize      uint64
		expectedSleep time.Duration
	}{
		"small file": {
			1 << 20, // 1 MB
			3 * time.Second,
		},
		"5 MB": {
			1 << 20 * 5, // 5 MB
			3 * time.Second,
		},
		"50 MB": {
			1 << 20 * 50, // 50 MB
			7 * time.Second,
		},
		"500 MB": {
			1 << 20 * 500, // 500 MB
			48 * time.Second,
		},
		"5 GB": {
			1 << 30 * 5, // 5 GB
			480 * time.Second,
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
