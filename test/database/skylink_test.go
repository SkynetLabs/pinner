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
	// Create the skylink.
	s, err := db.SkylinkCreate(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal("Failed to create a skylink:", err)
	}
	// Validate that the underlying skylink is the same.
	if s.Skylink != sl.String() {
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
	err = db.SkylinkServerAdd(ctx, sl, server)
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
}

// TestFetchAndLock tests the functionality of SkylinkFetchAndLockUnderpinned.
func TestFetchAndLock(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

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

	// We start with a minimum number of pinners set to 1.
	cfg.MinNumberOfPinners = 1

	// Try to fetch an underpinned skylink, expect none to be found.
	_, err = db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Create a new skylink.
	_, err = db.SkylinkCreate(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink, expect none to be found.
	_, err = db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Make sure it's pinned by fewer than the minimum number of servers.
	err = db.SkylinkServerRemove(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink, expect to find one.
	underpinned, err := db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
	if err != nil {
		t.Fatal(err)
	}
	if !underpinned.Equals(sl) {
		t.Fatalf("Expected to get '%s', got '%v'", sl, underpinned)
	}
	// Try to fetch an underpinned skylink from the name of a different server.
	// Expect to find none because the one we got before is now locked and
	// shouldn't be returned.
	_, err = db.SkylinkFetchAndLockUnderpinned(ctx, "different server", cfg.MinNumberOfPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Add a pinner.
	err = db.SkylinkServerAdd(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	err = db.SkylinkUnlock(ctx, sl, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink, expect none to be found.
	_, err = db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}

	// Increase the minimum number of pinners to two.
	cfg.MinNumberOfPinners = 2

	anotherServerName := "another server"
	thirdServerName := "third server"

	// Try to fetch an underpinned skylink, expect none to be found.
	// Out test skylink is underpinned but it's pinned by the given server, so
	// we expect it not to be returned.
	_, err = db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	// Try to fetch an underpinned skylink from the name of a different server.
	// Expect one to be found.
	_, err = db.SkylinkFetchAndLockUnderpinned(ctx, anotherServerName, cfg.MinNumberOfPinners)
	if err != nil {
		t.Fatal(err)
	}
	// Add a pinner.
	err = db.SkylinkServerAdd(ctx, sl, anotherServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to unlock the skylink from the name of a server that hasn't locked
	// it. Expect this to fail.
	err = db.SkylinkUnlock(ctx, sl, thirdServerName)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
	err = db.SkylinkUnlock(ctx, sl, anotherServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try to fetch an underpinned skylink with a third server name, expect none
	// to be found because our skylink is now properly pinned.
	_, err = db.SkylinkFetchAndLockUnderpinned(ctx, thirdServerName, cfg.MinNumberOfPinners)
	if !errors.Contains(err, database.ErrSkylinkNoExist) {
		t.Fatalf("Expected to get '%v', got '%v'", database.ErrSkylinkNoExist, err)
	}
}

// TestFetchAndLock ensures that SkylinkFetchAndLockUnderpinned will first check
// for files currently locked by the current server and only after that it will
// lock new ones.
func TestFetchAndLockOwnFirst(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

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

	// Create two skylinks from the name of another server and set the minimum
	// number of pinners to two, so these show up as underpinned.
	sl1 := test.RandomSkylink()
	sl2 := test.RandomSkylink()
	cfg.MinNumberOfPinners = 2
	otherServer := "other server"
	_, err = db.SkylinkCreate(ctx, sl1, otherServer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.SkylinkCreate(ctx, sl2, otherServer)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch and lock one of those.
	locked, err := db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
	if err != nil {
		t.Fatal(err)
	}
	// Add this server to the list of pinners, so we're sure that it's not being
	// randomly selected. This will ensure that the same skylink is being
	// returned because it's locked by the current server.
	err = db.SkylinkServerAdd(ctx, locked, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Try fetching another underpinned skylink before unlocking this one.
	// Expect to always get the same one until we unpin it.
	for i := 0; i < 10; i++ {
		newLocked, err := db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
		if err != nil {
			t.Fatal(err)
		}
		if newLocked != locked {
			t.Fatalf("Expected to get '%s', got '%s'", locked, newLocked)
		}
	}
	// Unlock it.
	err = db.SkylinkUnlock(ctx, locked, cfg.ServerName)
	if err != nil {
		t.Fatal(err)
	}
	// Fetch a new underpinned skylink. Expect it to be a different one.
	newLocked, err := db.SkylinkFetchAndLockUnderpinned(ctx, cfg.ServerName, cfg.MinNumberOfPinners)
	if err != nil {
		t.Fatal(err)
	}
	if newLocked == locked {
		t.Fatal("Fetched the same skylink again.")
	}
}
