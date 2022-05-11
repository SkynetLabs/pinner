package database

import (
	"context"
	"time"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/SkynetLabs/skyd/skymodules"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// ErrInvalidSkylink is returned when a client call supplies an invalid
	// skylink hash.
	ErrInvalidSkylink = errors.New("invalid skylink")
	// ErrSkylinkExists is returned when we try to create a skylink that already
	// exists.
	ErrSkylinkExists = errors.New("skylink already exists")
	// ErrSkylinkNotExist is returned when we try to get a skylink that doesn't
	// exist.
	ErrSkylinkNotExist = errors.New("skylink does not exist")
	// ErrNoSkylinksLocked is returned when we try to lock underpinned skylinks
	// for pinning but we fail to do so.
	ErrNoSkylinksLocked = errors.New("no skylinks locked")
	// LockDuration defines the duration of a database lock. We lock skylinks
	// while we are trying to pin them to a new server. The goal is to only
	// allow a single server to pin a given skylink at a time.
	LockDuration = 7 * time.Hour
)

type (
	// Skylink represents a skylink object in the DB.
	Skylink struct {
		ID      primitive.ObjectID `bson:"_id,omitempty"`
		Skylink string             `bson:"skylink"`
		Servers []string           `bson:"servers"`
		// Unpin informs all servers that there are no users pinning this
		// skylink and they should stop pinning it as well.
		Unpin       bool      `bson:"unpin"`
		LockedBy    string    `bson:"locked_by"`
		LockExpires time.Time `bson:"lock_expires"`
	}
)

// ConfigValue returns a cluster-wide configuration value, stored in the
// database.
func (db *DB) ConfigValue(ctx context.Context, key string) (string, error) {
	sr := db.staticDB.Collection(collConfig).FindOne(ctx, bson.M{"key": key})
	if sr.Err() != nil {
		return "", sr.Err()
	}
	var result = struct {
		Value string
	}{}
	err := sr.Decode(&result)
	if err != nil {
		return "", err
	}
	return result.Value, nil
}

// CreateSkylink inserts a new skylink into the DB. Returns an error if it
// already exists.
func (db *DB) CreateSkylink(ctx context.Context, skylink skymodules.Skylink, server string) (Skylink, error) {
	if server == "" {
		return Skylink{}, errors.New("invalid server name")
	}
	s := Skylink{
		Skylink: skylink.String(),
		Servers: []string{server},
	}
	ir, err := db.staticDB.Collection(collSkylinks).InsertOne(ctx, s)
	if mongo.IsDuplicateKeyError(err) {
		return Skylink{}, ErrSkylinkExists
	}
	if err != nil {
		return Skylink{}, err
	}
	s.ID = ir.InsertedID.(primitive.ObjectID)
	return s, nil
}

// FindSkylink fetches a skylink from the DB.
func (db *DB) FindSkylink(ctx context.Context, skylink skymodules.Skylink) (Skylink, error) {
	sr := db.staticDB.Collection(collSkylinks).FindOne(ctx, bson.M{"skylink": skylink.String()})
	if sr.Err() == mongo.ErrNoDocuments {
		return Skylink{}, ErrSkylinkNotExist
	}
	if sr.Err() != nil {
		return Skylink{}, sr.Err()
	}
	s := Skylink{}
	err := sr.Decode(&s)
	if err != nil {
		return Skylink{}, err
	}
	return s, nil
}

// MarkPinned marks a skylink as pinned (or no longer unpinned), meaning
// that Pinner should make sure it's pinned by the minimum number of servers.
func (db *DB) MarkPinned(ctx context.Context, skylink skymodules.Skylink) error {
	filter := bson.M{"skylink": skylink.String()}
	update := bson.M{"$set": bson.M{"unpin": false}}
	opts := options.Update().SetUpsert(true)
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update, opts)
	return err
}

// MarkUnpinned marks a skylink as unpinned, meaning that all servers
// should stop pinning it.
func (db *DB) MarkUnpinned(ctx context.Context, skylink skymodules.Skylink) error {
	filter := bson.M{"skylink": skylink.String()}
	update := bson.M{"$set": bson.M{"unpin": true}}
	opts := options.Update().SetUpsert(true)
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update, opts)
	return err
}

// AddServerForSkylink adds a new server to the list of servers known to be pinning
// this skylink. If the skylink does not already exist in the database it will
// be inserted. This operation is idempotent.
//
// The `markPinned` flag sets the `unpin` field of the skylink to false when
// raised but it doesn't set it to false when not raised. The reason for that is
// that it accommodates a specific use case - adding a server to the list of
// pinners of a given skylink will set the unpin field to false is we are doing
// that because we know that a user is pinning it but not so if we are running
// a server sweep and documenting which skylinks are pinned by this server.
func (db *DB) AddServerForSkylink(ctx context.Context, skylink skymodules.Skylink, server string, markPinned bool) error {
	filter := bson.M{"skylink": skylink.String()}
	var update bson.M
	if markPinned {
		update = bson.M{
			"$addToSet": bson.M{"servers": server},
			"$set":      bson.M{"unpin": false},
		}
	} else {
		update = bson.M{"$addToSet": bson.M{"servers": server}}
	}
	opts := options.Update().SetUpsert(true)
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update, opts)
	return err
}

// RemoveServerFromSkylink removes a server to the list of servers known to be
// pinning this skylink. If the skylink does not exist in the database it will
// not be inserted.
func (db *DB) RemoveServerFromSkylink(ctx context.Context, skylink skymodules.Skylink, server string) error {
	filter := bson.M{
		"skylink": skylink.String(),
		"servers": server,
	}
	update := bson.M{"$pull": bson.M{"servers": server}}
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update)
	return err
}

// FindAndLockUnderpinned fetches and locks a single underpinned skylink
// from the database. The method selects only skylinks which are not pinned by
// the given server.
//
// The MongoDB query is this:
// db.getCollection('skylinks').find({
//     "unpin": false,
//     "$expr": { "$lt": [{ "$size": "$servers" }, 2 ]},
//     "servers": { "$nin": [ "ro-tex.siasky.ivo.NOPE" ]},
//     "$or": [
//         { "lock_expires" : { "$exists": false }},
//         { "lock_expires" : { "$lt": new Date() }}
//     ]
// })
func (db *DB) FindAndLockUnderpinned(ctx context.Context, server string, minPinners int) (skymodules.Skylink, error) {
	filter := bson.M{
		"unpin": false,
		// Pinned by fewer than the minimum number of servers.
		"$expr": bson.M{"$lt": bson.A{bson.M{"$size": "$servers"}, minPinners}},
		// Not pinned by the given server.
		"servers": bson.M{"$nin": bson.A{server}},
		// Unlocked.
		"$or": bson.A{
			bson.M{"lock_expires": bson.M{"$exists": false}},
			bson.M{"lock_expires": bson.M{"$lt": time.Now().UTC().Truncate(time.Millisecond)}},
		},
	}
	update := bson.M{
		"$set": bson.M{
			"locked_by":    server,
			"lock_expires": time.Now().UTC().Add(LockDuration).Truncate(time.Millisecond),
		},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	sr := db.staticDB.Collection(collSkylinks).FindOneAndUpdate(ctx, filter, update, opts)
	if sr.Err() == mongo.ErrNoDocuments {
		return skymodules.Skylink{}, ErrSkylinkNotExist
	}
	if sr.Err() != nil {
		return skymodules.Skylink{}, sr.Err()
	}
	var result struct {
		Skylink string
	}
	err := sr.Decode(&result)
	if err != nil {
		return skymodules.Skylink{}, errors.AddContext(err, "failed to decode result")
	}
	return SkylinkFromString(result.Skylink)
}

// UnlockSkylink removes the lock on the skylink put while we're trying to pin
// it to a new server.
func (db *DB) UnlockSkylink(ctx context.Context, skylink skymodules.Skylink, server string) error {
	filter := bson.M{
		"skylink":   skylink.String(),
		"locked_by": server,
	}
	update := bson.M{
		"$set": bson.M{
			"locked_by":    "",
			"lock_expires": time.Time{},
		},
	}
	ur, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update)
	if errors.Contains(err, mongo.ErrNoDocuments) || ur.ModifiedCount == 0 {
		return ErrNoSkylinksLocked
	}
	return err
}

// SkylinkFromString converts a string to a Skylink.
func SkylinkFromString(s string) (skymodules.Skylink, error) {
	var sl skymodules.Skylink
	err := sl.LoadString(s)
	if err != nil {
		return skymodules.Skylink{}, err
	}
	return sl, nil
}
