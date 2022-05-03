package database

import (
	"context"
	"testing"

	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/test"
	"gitlab.com/NebulousLabs/errors"
)

// TestSkylink is a comprehensive test suite that covers the entire
// functionality of the Skylink database type.
func TestSkylink(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	cfg, err := test.LoadTestConfig()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	dbName := test.DBNameForTest(t.Name())
	db, err := test.NewDatabase(ctx, dbName)
	if err != nil {
		t.Fatal(err)
	}

	sl := test.RandomSkylink()

	// Fetch the skylink from the DB. Expect ErrSkylinkNoExist.
	_, err = db.SkylinkFetch(ctx, sl)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected error %v, got %v.", database.ErrSkylinkNoExist, err)
	}
	// Try to create an invalid skylink.
	_, err = db.SkylinkCreate(ctx, "this is not a valid skylink", cfg.ServerName)
	if err == nil {
		t.Fatal("Managed to create an invalid skylink.")
	}
	// Create the skylink.
	s, err := db.SkylinkCreate(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal("Failed to create a skylink:", err)
	}
	// Validate that the underlying skylink is the same.
	if s.Skylink != sl {
		t.Fatalf("Expected skylink '%s', got '%s'", sl, s.Skylink)
	}
	// Add the skylink again, expect this to fail with ErrSkylinkExists.
	otherServer := "second create"
	_, err = db.SkylinkCreate(ctx, sl, otherServer)
	if !errors.Contains(err, database.ErrSkylinkExists) {
		t.Fatalf("Expected '%v', got '%v'", database.ErrSkylinkExists, err)
	}
	// Clean up.
	err = db.SkylinkServerRemove(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}

	// Add a new server to the list.
	server := "new server"
	err = db.SkylinkServerAdd(ctx, sl, server, false)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.SkylinkFetch(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the new server was added to the list.
	if s.Servers[0] != server && s.Servers[1] != server {
		t.Fatalf("Expected to find '%s' in the list, got '%v'", server, s.Servers)
	}
	// Remove a server from the list.
	err = db.SkylinkServerRemove(ctx, sl, server)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.SkylinkFetch(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the new server was added to the list.
	if len(s.Servers) != 1 || s.Servers[0] != cfg.ServerName {
		t.Fatalf("Expected to find only '%s' in the list, got '%v'", cfg.ServerName, s.Servers)
	}
	// Mark the file as unpinned.
	err = db.SkylinkMarkUnpinned(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.SkylinkFetch(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Unpin {
		t.Fatal("Expected the skylink to be unpinned.")
	}
	// Mark the file as pinned again.
	err = db.SkylinkMarkPinned(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.SkylinkFetch(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if s.Unpin {
		t.Fatal("Expected the skylink to not be unpinned.")
	}
	// Mark the skylink as unpinned again.
	err = db.SkylinkMarkUnpinned(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Add a server to it with the `markUnpinned` set to false.
	// Expect the skylink to remain unpinned.
	err = db.SkylinkServerAdd(ctx, sl, "new server pin false", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.SkylinkFetch(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Unpin {
		t.Fatal("Expected the skylink to be unpinned.")
	}
	// Add a server to the skylink with `markUnpinned` set to true.
	// Expect the skylink to not be unpinned.
	err = db.SkylinkServerAdd(ctx, sl, "new server pin true", true)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.SkylinkFetch(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if s.Unpin {
		t.Fatal("Expected the skylink to not be unpinned.")
	}
}
