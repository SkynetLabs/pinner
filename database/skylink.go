package database

import (
	"context"

	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// ErrSkylinkExists is returned when we try to create a skylink that already
	// exists.
	ErrSkylinkExist = errors.New("skylink already exists")
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
	servers := []string{}
	if server != "" {
		servers = append(servers, server)
	}
	s := Skylink{
		Skylink: sl,
		Servers: servers,
	}
	ir, err := db.staticDB.Collection(collSkylinks).InsertOne(ctx, s)
	if mongo.IsDuplicateKeyError(err) {
		return Skylink{}, ErrSkylinkExist
	}
	if err != nil {
		return Skylink{}, err
	}
	s.ID = ir.InsertedID.(primitive.ObjectID)
	return s, nil
}

// Skylink gets a skylink from the DB.
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

// SkylinkServerRemoveremoves a server to the list of servers known to be
// pinning this skylink. If the skylink does not exist in the database it will
// not be inserted.
func (db *DB) SkylinkServerRemove(ctx context.Context, sl Skylink, server string) error {
	filter := bson.M{
		"skylink": sl,
		"servers": server,
	}
	update := bson.M{"$pull": bson.M{"servers": server}}
	_, err := db.staticDB.Collection(collSkylinks).UpdateOne(ctx, filter, update)
	return err
}
