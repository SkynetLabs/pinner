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
	t.Parallel()

	cfg, err := test.LoadTestConfig()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := test.NewDatabase(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}

	sl := test.RandomSkylink()

	// Fetch the skylink from the DB. Expect ErrSkylinkNoExist.
	_, err = db.FindSkylink(ctx, sl)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected error %v, got %v.", database.ErrSkylinkNoExist, err)
	}
	// Create the skylink.
	s, err := db.CreateSkylink(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal("Failed to create a skylink:", err)
	}
	// Validate that the underlying skylink is the same.
	if s.Skylink != sl.String() {
		t.Fatalf("Expected skylink '%s', got '%s'", sl, s.Skylink)
	}
	// Add the skylink again, expect this to fail with ErrSkylinkExists.
	otherServer := "second create"
	_, err = db.CreateSkylink(ctx, sl, otherServer)
	if !errors.Contains(err, database.ErrSkylinkExists) {
		t.Fatalf("Expected '%v', got '%v'", database.ErrSkylinkExists, err)
	}
	// Clean up.
	err = db.RemoveServerFromSkylink(ctx, sl, otherServer)
	if err != nil {
		t.Fatal(err)
	}

	// Add a new server to the list.
	server := "new server"
	err = db.AddServerForSkylink(ctx, sl, server, false)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.FindSkylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the new server was added to the list.
	if s.Servers[0] != server && s.Servers[1] != server {
		t.Fatalf("Expected to find '%s' in the list, got '%v'", server, s.Servers)
	}
	// Remove a server from the list.
	err = db.RemoveServerFromSkylink(ctx, sl, server)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.FindSkylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the new server was added to the list.
	if len(s.Servers) != 1 || s.Servers[0] != cfg.ServerName {
		t.Fatalf("Expected to find only '%s' in the list, got '%v'", cfg.ServerName, s.Servers)
	}
	// Mark the file as unpinned.
	err = db.MarkUnpinned(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.FindSkylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Unpin {
		t.Fatal("Expected the skylink to be unpinned.")
	}
	// Mark the file as pinned again.
	err = db.MarkPinned(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.FindSkylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if s.Unpin {
		t.Fatal("Expected the skylink to be pinned.")
	}
	// Mark the skylink as unpinned again.
	err = db.MarkUnpinned(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	// Add a server to it with the `markUnpinned` set to false.
	// Expect the skylink to remain unpinned.
	err = db.AddServerForSkylink(ctx, sl, "new server pin false", false)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.FindSkylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Unpin {
		t.Fatal("Expected the skylink to be unpinned.")
	}
	// Add a server to the skylink with `markUnpinned` set to true.
	// Expect the skylink to be pinned.
	err = db.AddServerForSkylink(ctx, sl, "new server pin true", true)
	if err != nil {
		t.Fatal(err)
	}
	s, err = db.FindSkylink(ctx, sl)
	if err != nil {
		t.Fatal(err)
	}
	if s.Unpin {
		t.Fatal("Expected the skylink to be pinned.")
	}
}

// TestFindAndLock tests the functionality of FindAndLockUnderpinned.
func TestFindAndLock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	cfg, err := test.LoadTestConfig()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := test.NewDatabase(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}

	sl := test.RandomSkylink()

	// We start with a minimum number of pinners set to 1.
	cfg.MinPinners = 1

	// Try to fetch an underpinned skylink, expect none to be found.
	_, err = db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Create a new skylink.
	_, err = db.CreateSkylink(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink, expect none to be found.
	_, err = db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Make sure it's pinned by fewer than the minimum number of servers.
	err = db.RemoveServerFromSkylink(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink, expect to find one.
	underpinned, err := db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if err != nil {
		t.Fatal(err)
	}
	if !underpinned.Equals(sl) {
		t.Fatalf("Expected to get '%s', got '%v'", sl, underpinned)
	}
	// Try to fetch an underpinned skylink from the name of a different server.
	// Expect to find none because the one we got before is now locked and
	// shouldn't be returned.
	_, err = db.FindAndLockUnderpinned(ctx, "different server", cfg.MinPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Add a pinner.
	err = db.AddServerForSkylink(ctx, sl, cfg.ServerName, false)
	if err != nil {
		t.Fatal(err)
	}
	err = db.UnlockSkylink(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink, expect none to be found.
	_, err = db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}

	// Increase the minimum number of pinners to two.
	cfg.MinPinners = 2

	anotherServerName := "another server"
	thirdServerName := "third server"

	// Try to fetch an underpinned skylink, expect none to be found.
	// Out test skylink is underpinned but it's pinned by the given server, so
	// we expect it not to be returned.
	_, err = db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Try to fetch an underpinned skylink from the name of a different server.
	// Expect one to be found.
	_, err = db.FindAndLockUnderpinned(ctx, anotherServerName, cfg.MinPinners)
	if err != nil {
		t.Fatal(err)
	}
	// Add a pinner.
	err = db.AddServerForSkylink(ctx, sl, anotherServerName, false)
	if err != nil {
		t.Fatal(err)
	}
	// Try to unlock the skylink from the name of a server that hasn't locked
	// it. Expect this to fail.
	err = db.UnlockSkylink(ctx, sl, thirdServerName)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	err = db.UnlockSkylink(ctx, sl, anotherServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink with a third server name, expect none
	// to be found because our skylink is now properly pinned.
	_, err = db.FindAndLockUnderpinned(ctx, thirdServerName, cfg.MinPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
}

// TestFindAndLock ensures that FindAndLockUnderpinned will first check
// for files currently locked by the current server and only after that it will
// lock new ones.
func TestFindAndLockOwnFirst(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	cfg, err := test.LoadTestConfig()
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	db, err := test.NewDatabase(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}

	// Create two skylinks from the name of another server and set the minimum
	// number of pinners to two, so these show up as underpinned.
	sl1 := test.RandomSkylink()
	sl2 := test.RandomSkylink()
	cfg.MinPinners = 2
	otherServer := "other server"
	_, err = db.CreateSkylink(ctx, sl1, otherServer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.CreateSkylink(ctx, sl2, otherServer)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch and lock one of those.
	locked, err := db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if err != nil {
		t.Fatal(err)
	}
	// Add this server to the list of pinners, so we're sure that it's not being
	// randomly selected. This will ensure that the same skylink is being
	// returned because it's locked by the current server.
	err = db.AddServerForSkylink(ctx, locked, cfg.ServerName, false)
	if err != nil {
		t.Fatal(err)
	}
	// Try fetching another underpinned skylink before unlocking this one.
	// Expect to get a different one.
	newLocked, err := db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if err != nil {
		t.Fatal(err)
	}
	if newLocked == locked {
		t.Fatal("Expected to get a different skylink.")
	}
	// Unlock it.
	err = db.UnlockSkylink(ctx, locked, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch a new underpinned skylink. Expect it to fail because we've run out
	// of underpinned skylinks.
	newLocked, err = db.FindAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
}
