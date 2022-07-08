package database

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/skynetlabs/pinner/test"
	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/NebulousLabs/fastrand"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestSetConfigValue ensures that we properly set database config values.
func TestSetConfigValue(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Parallel()

	ctx := context.Background()
	db, err := test.NewDatabase(ctx, t.Name())
	if err != nil {
		t.Fatal(err)
	}

	key := hex.EncodeToString(fastrand.Bytes(16))
	val := hex.EncodeToString(fastrand.Bytes(16))

	// Ensure we don't have a value in the DB.
	_, err = db.ConfigValue(ctx, key)
	if !errors.Contains(err, mongo.ErrNoDocuments) {
		t.Fatalf("Expected '%v', got '%v'", mongo.ErrNoDocuments, err)
	}
	// Set the value.
	err = db.SetConfigValue(ctx, key, val)
	if err != nil {
		t.Fatal(err)
	}
	v, err := db.ConfigValue(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if v != val {
		t.Fatalf("Expected '%s', got '%s'", val, v)
	}
}
