package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/test"
	"gitlab.com/NebulousLabs/errors"
)

// subtest defines the structure of a subtest
type subtest struct {
	name string
	test func(t *testing.T, tt *test.Tester)
}

// TestHandlers is a meta test that sets up a test instance of pinner and runs
// a suite of tests that ensure all handlers behave as expected.
func TestHandlers(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	tt, err := test.NewTester(t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := tt.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close tester"))
		}
	}()

	// Specify subtests to run
	tests := []subtest{
		{name: "Health", test: testHandlerHealthGET},
		{name: "Pin", test: testHandlerPinPOST},
		{name: "Unpin", test: testHandlerUnpinPOST},
		{name: "Sweep", test: testHandlerSweep},
	}

	// Run subtests
	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			tst.test(t, tt)
		})
	}
}

// testHandlerHealthGET tests the "GET /health" handler.
func testHandlerHealthGET(t *testing.T, tt *test.Tester) {
	status, _, err := tt.HealthGET()
	if err != nil {
		t.Fatal(err)
	}
	// DBAlive should never be false because if we couldn't reach the DB, we
	// wouldn't have made it this far in the test.
	if !status.DBAlive {
		t.Fatal("DB down.")
	}
}

// testHandlerPinPOST tests "POST /pin"
func testHandlerPinPOST(t *testing.T, tt *test.Tester) {
	sl := test.RandomSkylink()

	// Pin an invalid skylink.
	_, err := tt.PinPOST("this is not a skylink")
	if err == nil || !strings.Contains(err.Error(), database.ErrInvalidSkylink.Error()) {
		t.Fatalf("Expected error '%s', got '%v'", database.ErrInvalidSkylink, err)
	}
	// Pin a valid skylink.
	status, err := tt.PinPOST(sl.String())
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}

	// Mark the skylink as unpinned and pin it again.
	// Expect it to no longer be unpinned.
	err = tt.DB.MarkUnpinned(tt.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	status, err = tt.PinPOST(sl.String())
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	slNew, err := tt.DB.FindSkylink(tt.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if slNew.Unpin {
		t.Fatal("Expected the skylink to no longer be marked as unpinned.")
	}
}

// testHandlerUnpinPOST tests "POST /unpin"
func testHandlerUnpinPOST(t *testing.T, tt *test.Tester) {
	sl := test.RandomSkylink()

	// Unpin an invalid skylink.
	_, err := tt.UnpinPOST("this is not a skylink")
	if err == nil || !strings.Contains(err.Error(), database.ErrInvalidSkylink.Error()) {
		t.Fatalf("Expected error '%s', got '%v'", database.ErrInvalidSkylink, err)
	}
	// Pin a valid skylink.
	status, err := tt.PinPOST(sl.String())
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	// Unpin the skylink.
	status, err = tt.UnpinPOST(sl.String())
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	// Make sure the skylink is marked as unpinned.
	slNew, err := tt.DB.FindSkylink(tt.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !slNew.Unpin {
		t.Fatal("Expected the skylink to be marked as unpinned.")
	}
	// Unpin a valid skylink that's not in the DB, yet.
	sl2 := test.RandomSkylink()
	status, err = tt.UnpinPOST(sl2.String())
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	// Make sure the skylink is marked as unpinned.
	sl2New, err := tt.DB.FindSkylink(tt.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !sl2New.Unpin {
		t.Fatal("Expected the skylink to be marked as unpinned.")
	}
}

// testHandlerSweep tests both "POST /sweep" and "GET /sweep/status"
func testHandlerSweep(t *testing.T, tt *test.Tester) {
	// Check status. Expect zero value, no error.
	sweepStatus, code, err := tt.SweepStatusGET()
	if err != nil || code != http.StatusOK {
		t.Fatalf("Unexpected status code or error: %d %+v", code, err)
	}
	if sweepStatus.InProgress || !sweepStatus.StartTime.Equal(time.Time{}) {
		t.Fatalf("Unexpected sweep detected: %+v", sweepStatus)
	}
	// Start a sweep. Expect to return immediately with a 202.
	sweepReqTime := time.Now().UTC()
	sr, code, err := tt.SweepPOST()
	if err != nil || code != http.StatusAccepted {
		t.Fatalf("Unexpected status code or error: %d %+v", code, err)
	}
	if sr.Href != "/sweep/status" {
		t.Fatalf("Unexpected href: '%s'", sr.Href)
	}
	// Make sure that the call returned quickly, i.e. it didn't wait for the
	// sweep to end but rather returned immediately and let the sweep run in the
	// background. Rebuilding the cache alone takes 100ms.
	if time.Now().UTC().Add(-50 * time.Millisecond).After(sweepReqTime) {
		t.Fatal("Call to status took too long.")
	}
	// Check status. Expect a sweep in progress.
	sweepStatus, code, err = tt.SweepStatusGET()
	if err != nil || code != http.StatusOK {
		t.Fatalf("Unexpected status code or error: %d %+v", code, err)
	}
	if !sweepStatus.InProgress {
		t.Fatal("Expected to detect a sweep")
	}
	// Start a sweep.
	_, code, err = tt.SweepPOST()
	if err != nil || code != http.StatusAccepted {
		t.Fatalf("Unexpected status code or error: %d %+v", code, err)
	}

	// Check status. Expect the sweep start time to be the same as before, i.e.
	// no new sweep has been kicked off.
	initialSweepStartTime := sweepStatus.StartTime
	sweepStatus, code, err = tt.SweepStatusGET()
	if err != nil || code != http.StatusOK {
		t.Fatalf("Unexpected status code or error: %d %+v", code, err)
	}
	if !sweepStatus.InProgress {
		t.Fatal("Expected to detect a sweep")
	}
	if !sweepStatus.StartTime.Equal(initialSweepStartTime) {
		t.Fatalf("Expected the start time of the current scan to match the start time of the first scan we kicked off. Expected %v, got %v", initialSweepStartTime, sweepStatus.StartTime)
	}
}
