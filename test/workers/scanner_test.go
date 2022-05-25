package workers

import (
	"context"
	"testing"
	"time"

	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/skyd"
	"github.com/skynetlabs/pinner/test"
	"github.com/skynetlabs/pinner/workers"
	"gitlab.com/NebulousLabs/errors"
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
	scanner := workers.NewScanner(db, test.NewDiscardLogger(), cfg.MinPinners, cfg.ServerName, skydcm)
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
	// Sleep for two cycles.
	time.Sleep(2 * workers.SleepBetweenScans())
	// Make sure the skylink isn't pinned on the local (mock) skyd.
	if skydcm.IsPinning(sl.String()) {
		t.Fatal("We didn't expect skyd to be pinning this.")
	}
	// Remove the other server, making the file underpinned.
	err = db.RemoveServerFromSkylink(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}

	// Wait - the skylink should be picked up and pinned on the local skyd.
	time.Sleep(3 * workers.SleepBetweenScans())

	// Make sure the skylink is pinned on the local (mock) skyd.
	if !skydcm.IsPinning(sl.String()) {
		t.Fatal("We expected skyd to be pinning this.")
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
	scanner := workers.NewScanner(db, test.NewDiscardLogger(), cfg.MinPinners, cfg.ServerName, skydcm)
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
	// Sleep for two cycles.
	time.Sleep(2 * workers.SleepBetweenScans())
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
	time.Sleep(3 * workers.SleepBetweenScans())

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

	// Verify skyd gets the pin.
	//
	// Wait enough time for the pinner to pick up the new value for dry_run and
	// rescan.
	time.Sleep(10 * workers.SleepBetweenScans())

	// Make sure the skylink is pinned on the local (mock) skyd.
	if !skydcm.IsPinning(sl.String()) {
		t.Fatal("We expected skyd to be pinning this.")
	}
}
