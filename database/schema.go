package database

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// schema defines a mapping between a collection name and the indexes that
// must exist for that collection.
var schema = map[string][]mongo.IndexModel{
	collSkylinks: {
		{
			Keys:    bson.D{{"skylink", 1}},
			Options: options.Index().SetName("skylink").SetUnique(true),
		},
		{
			Keys:    bson.D{{"lock_expires", 1}},
			Options: options.Index().SetName("lock_expires"),
		},
		{
			Keys:    bson.D{{"servers", 1}},
			Options: options.Index().SetName("servers"),
		},
	},
}
