package workers

import (
	"context"
	"testing"
	"time"

	"github.com/skynetlabs/pinner/test"
	"github.com/skynetlabs/pinner/test/mocks"
	"github.com/skynetlabs/pinner/workers"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/threadgroup"
)

// TestScanner ensures that Scanner does its job.
func TestScanner(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := test.NewDatabase(ctx, dbName)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := test.LoadTestConfig()
	if err != nil {
		t.Fatal(err)
	}
	skydcm := &mocks.SkydClientMock{}
	var tg threadgroup.ThreadGroup
	scanner := workers.NewScanner(db, test.NewDiscardLogger(), cfg.MinNumberOfPinners, cfg.ServerName, skydcm, &tg)
	defer func() {
		if e := tg.Stop(); e != nil {
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
	time.Sleep(2 * workers.SleepBetweenScans)
	// Make sure the skylink isn't pinned on the local (mock) skyd.
	if skydcm.IsPinning(sl.String()) {
		t.Fatal("We didn't expect skyd to be pinning this.")
	}
	// Remove the other server, making the file underpinned.
	err = db.RemoveServerFromSkylink(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}
	// Wait for one cycle - the skylink should be picked up and pinned on the
	// local skyd.
	time.Sleep(workers.SleepBetweenScans)
	// Make sure the skylink is pinned on the local (mock) skyd.
	if !skydcm.IsPinning(sl.String()) {
		t.Fatal("We expected skyd to be pinning this.")
	}
}