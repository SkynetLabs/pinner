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
	// ErrSkylinkNoExist is returned when we try to get a skylink that doesn't
	// exist.
	ErrSkylinkNoExist = errors.New("skylink does not exist")

	// LockDuration defines the duration of a database lock. We lock skylinks
	// while we are trying to pin them to a new server. The goal is to only
	// allow a single server to pin a given skylink at a time.
	LockDuration = 24 * time.Hour
)

type (
	// Skylink represents a skylink object in the DB.
	// The Unpin field instructs all servers, currently pinning this skylink,
	// that there are no users pinning and the servers should unpin it as well.
	Skylink struct {
		ID          primitive.ObjectID `bson:"_id,omitempty"`
		Skylink     string             `bson:"skylink"`
		Servers     []string           `bson:"servers"`
		Unpin       bool               `bson:"unpin"`
		LockedBy    string             `bson:"locked_by"`
		LockExpires time.Time          `bson:"lock_expires"`
	}
)

// SkylinkCreate inserts a new skylink into the DB. Returns an error if it
// already exists.
func (db *DB) SkylinkCreate(ctx context.Context, skylink skymodules.Skylink, server string) (Skylink, error) {
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

// SkylinkFetch fetches a skylink from the DB.
func (db *DB) SkylinkFetch(ctx context.Context, skylink skymodules.Skylink) (Skylink, error) {
	sr := db.staticDB.Collection(collSkylinks).FindOne(ctx, bson.M{"skylink": skylink.String()})
	if sr.Err() == mongo.ErrNoDocuments {
		return Skylink{}, ErrSkylinkNoExist
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

// SkylinkMarkPinned marks a skylink as pinned (or no longer unpinned), meaning
// that Pinner should make sure it's pinned by the minimum number of servers.
func (db *DB) SkylinkMarkPinned(ctx context.Context, skylink skymodules.Skylink) error {
	filter := bson.M{"skylink": skylink.String()}
	update := bson.M{"$set": bson.M{"unpin": false}}
	opts := options.Update().SetUpsert(true)
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update, opts)
	return err
}

// SkylinkMarkUnpinned marks a skylink as unpinned, meaning that all servers
// should stop pinning it.
func (db *DB) SkylinkMarkUnpinned(ctx context.Context, skylink skymodules.Skylink) error {
	filter := bson.M{"skylink": skylink.String()}
	update := bson.M{"$set": bson.M{"unpin": true}}
	opts := options.Update().SetUpsert(true)
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update, opts)
	return err
}

// SkylinkServerAdd adds a new server to the list of servers known to be pinning
// this skylink. If the skylink does not already exist in the database it will
// be inserted. This operation is idempotent.
func (db *DB) SkylinkServerAdd(ctx context.Context, skylink skymodules.Skylink, server string) error {
	filter := bson.M{"skylink": skylink.String()}
	update := bson.M{"$addToSet": bson.M{"servers": server}}
	opts := options.Update().SetUpsert(true)
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update, opts)
	return err
}

// SkylinkServerRemove removes a server to the list of servers known to be
// pinning this skylink. If the skylink does not exist in the database it will
// not be inserted.
func (db *DB) SkylinkServerRemove(ctx context.Context, skylink skymodules.Skylink, server string) error {
	filter := bson.M{
		"skylink": skylink.String(),
		"servers": server,
	}
	update := bson.M{"$pull": bson.M{"servers": server}}
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update)
	return err
}

// SkylinkFetchAndLockUnderpinned fetches and locks a single underpinned skylink
// from the database. The method selects only skylinks which are not pinned by
// the given server.
//
// The MongoDB query is this:
// db.getCollection('skylinks').find({
//     "$expr": { "$lt": [{ "$size": "$servers" }, 2 ]},
//     "servers": { "$nin": [ "ro-tex.siasky.ivo.NOPE" ]},
//     "$or": [
//         { "lock_expires" : { "$exists": false }},
//         { "lock_expires" : { "$lt": new Date() }}
//     ]
// })
func (db *DB) SkylinkFetchAndLockUnderpinned(ctx context.Context, server string, minPinners int) (skymodules.Skylink, error) {
	// First try to fetch a skylink which is locked by the current server. This
	// is our mechanism for proactively recovering from files being left locked
	// after a server crash.
	sl, err := db.skylinkFetchLocked(ctx, server)
	if err == nil {
		return sl, nil
	}
	// No files locked by the current server were found, look for unlocked ones.
	filter := bson.M{
		"unpin": false,
		// Pinned by fewer than the minimum number of servers.
		"$expr": bson.M{"$lt": bson.A{bson.M{"$size": "$servers"}, minPinners}},
		// Not pinned by the given server.
		"servers": bson.M{"$nin": bson.A{server}},
		// Unlocked.
		"$or": bson.A{
			bson.M{"lock_expires": bson.M{"$exists": false}},
			bson.M{"lock_expires": bson.M{"$lt": time.Now().UTC()}},
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
		return skymodules.Skylink{}, ErrSkylinkNoExist
	}
	if sr.Err() != nil {
		return skymodules.Skylink{}, sr.Err()
	}
	var result struct {
		Skylink string
	}
	err = sr.Decode(&result)
	if err != nil {
		return skymodules.Skylink{}, errors.AddContext(err, "failed to decode result")
	}
	return SkylinkFromString(result.Skylink)
}

// skylinkFetchLocked fetches a skylink that's locked by the current server, if
// one exists.
func (db *DB) skylinkFetchLocked(ctx context.Context, server string) (skymodules.Skylink, error) {
	filter := bson.M{
		"locked_by":    server,
		"lock_expires": bson.M{"$gt": time.Now().UTC()},
		"unpin":        false,
	}
	sr := db.staticDB.Collection(collSkylinks).FindOne(ctx, filter)
	if sr.Err() != nil {
		return skymodules.Skylink{}, sr.Err()
	}
	var result struct {
		Skylink string
	}
	err := sr.Decode(&result)
	if err != nil {
		return skymodules.Skylink{}, err
	}
	return SkylinkFromString(result.Skylink)
}

// SkylinkUnlock removes the lock on the skylink put while we're trying to pin
// it to a new server.
func (db *DB) SkylinkUnlock(ctx context.Context, skylink skymodules.Skylink, server string) error {
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
		return ErrSkylinkNoExist
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
