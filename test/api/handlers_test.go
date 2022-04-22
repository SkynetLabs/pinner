package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/test"
	"gitlab.com/NebulousLabs/errors"
)

type subtest struct {
	name string
	test func(t *testing.T, at *test.Tester)
}

// TestHandlers is a meta test that sets up a test instance of pinner and runs
// a suite of tests that ensure all handlers behave as expected.
func TestHandlers(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	if conf.ServerName == "" {
		conf.ServerName = test.TestServerName
	}

	dbName := test.DBNameForTest(t.Name())
	at, err := test.NewTester(dbName)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if errClose := at.Close(); errClose != nil {
			t.Error(errors.AddContext(errClose, "failed to close account tester"))
		}
	}()

	// Specify subtests to run
	tests := []subtest{
		{name: "Health", test: testHandlerHealthGET},
		{name: "Pin", test: testHandlerPinPOST},
	}

	// Run subtests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t, at)
		})
	}
}

// testHandlerHealthGET tests the /health handler.
func testHandlerHealthGET(t *testing.T, at *test.Tester) {
	status, _, err := at.HealthGET()
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
func testHandlerPinPOST(t *testing.T, at *test.Tester) {
	sl := test.RandomSkylink()

	// Pin an invalid skylink.
	_, err := at.PinPOST("this is not a skylink")
	if err == nil || !strings.Contains(err.Error(), database.ErrInvalidSkylink.Error()) {
		t.Fatalf("Expected error '%s', got '%v'", database.ErrInvalidSkylink, err)
	}
	// Pin a valid skylink.
	status, err := at.PinPOST(sl)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}

	// Mark the skylink as unpinned and pin it again.
	// Expect it to no longer be unpinned.
	err = at.DB.SkylinkMarkUnpinned(at.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	status, err = at.PinPOST(sl)
	if err != nil || status != http.StatusNoContent {
		t.Fatal(status, err)
	}
	slNew, err := at.DB.SkylinkFetch(at.Ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if slNew.Unpin {
		t.Fatal("Expected the skylink to no longer be marked as unpinned.")
	}
}
