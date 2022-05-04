package api

import (
	"net/http"
	"strings"
	"testing"

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

	dbName := test.DBNameForTest(t.Name())
	tt, err := test.NewTester(dbName)
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
	}

	// Run subtests
	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			tst.test(t, tt)
		})
	}
}

// testHandlerHealthGET tests the /health handler.
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
	status, err := tt.PinPOST(sl)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}

	// Mark the skylink as unpinned and pin it again.
	// Expect it to no longer be unpinned.
	err = tt.DB.MarkUnpinned(tt.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	status, err = tt.PinPOST(sl)
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
func testHandlerUnpinPOST(t *testing.T, at *test.Tester) {
	sl := test.RandomSkylink()

	// Unpin an invalid skylink.
	_, err := at.UnpinPOST("this is not a skylink")
	if err == nil || !strings.Contains(err.Error(), database.ErrInvalidSkylink.Error()) {
		t.Fatalf("Expected error '%s', got '%v'", database.ErrInvalidSkylink, err)
	}
	// Pin a valid skylink.
	status, err := at.PinPOST(sl)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	// Unpin the skylink.
	status, err = at.UnpinPOST(sl)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	// Make sure the skylink is marked as unpinned.
	slNew, err := at.DB.FindSkylink(at.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !slNew.Unpin {
		t.Fatal("Expected the skylink to be marked as unpinned.")
	}
	// Unpin a valid skylink that's not in the DB, yet.
	sl2 := test.RandomSkylink()
	status, err = at.UnpinPOST(sl2)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	// Make sure the skylink is marked as unpinned.
	sl2New, err := at.DB.FindSkylink(at.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !sl2New.Unpin {
		t.Fatal("Expected the skylink to be marked as unpinned.")
	}
}
