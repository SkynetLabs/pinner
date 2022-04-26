package database

import (
	"context"

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
)

type (
	// Skylink represents a skylink object in the DB.
	Skylink struct {
		ID      primitive.ObjectID `bson:"_id,omitempty"`
		Skylink string             `bson:"skylink"`
		Servers []string           `bson:"servers"`
	}
)

// SkylinkCreate inserts a new skylink into the DB. Returns an error if it
// already exists.
func (db *DB) SkylinkCreate(ctx context.Context, sl string, server string) (Skylink, error) {
	var skylink skymodules.Skylink
	err := skylink.LoadString(sl)
	if err != nil {
		return Skylink{}, ErrInvalidSkylink
	}
	if server == "" {
		return Skylink{}, errors.New("invalid server name")
	}
	s := Skylink{
		Skylink: sl,
		Servers: []string{server},
	}
	ir, err := db.staticDB.Collection(collSkylinks).InsertOne(ctx, s)
	if mongo.IsDuplicateKeyError(err) {
		err = db.SkylinkServerAdd(ctx, sl, server)
		if err != nil {
			return Skylink{}, err
		}
		return db.SkylinkFetch(ctx, sl)
	}
	if err != nil {
		return Skylink{}, err
	}
	s.ID = ir.InsertedID.(primitive.ObjectID)
	return s, nil
}

// SkylinkFetch fetches a skylink from the DB.
func (db *DB) SkylinkFetch(ctx context.Context, sl string) (Skylink, error) {
	sr := db.staticDB.Collection(collSkylinks).FindOne(ctx, bson.M{"skylink": sl})
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

// SkylinkServerAdd adds a new server to the list of servers known to be pinning
// this skylink. If the skylink does not already exist in the database it will
// be inserted. This operation is idempotent.
func (db *DB) SkylinkServerAdd(ctx context.Context, sl string, server string) error {
	filter := bson.M{"skylink": sl}
	update := bson.M{"$addToSet": bson.M{"servers": server}}
	opts := options.UpdateOptions{}
	opts.SetUpsert(true)
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update, &opts)
	return err
}

// SkylinkServerRemove removes a server to the list of servers known to be
// pinning this skylink. If the skylink does not exist in the database it will
// not be inserted.
func (db *DB) SkylinkServerRemove(ctx context.Context, sl string, server string) error {
	filter := bson.M{
		"skylink": sl,
		"servers": server,
	}
	update := bson.M{"$pull": bson.M{"servers": server}}
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update)
	return err
}
