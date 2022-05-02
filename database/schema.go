package database

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// schema returns a mapping between a collection name and the indexes that
// must exist for that collection.
//
// We return a map literal instead of using a global variable because the global
// variable causes data races when multiple tests are creating their own
// databases and are iterating over the schema at the same time.
func schema() map[string][]mongo.IndexModel {
	return map[string][]mongo.IndexModel{
		collSkylinks: {
			{
				Keys:    bson.D{{"skylink", 1}},
				Options: options.Index().SetName("skylink").SetUnique(true),
			},
			{
				Keys:    bson.D{{"locked_by", 1}},
				Options: options.Index().SetName("locked_by"),
			},
			{
				Keys:    bson.D{{"lock_expires", 1}},
				Options: options.Index().SetName("lock_expires"),
			},
			{
				Keys:    bson.D{{"servers", 1}},
				Options: options.Index().SetName("servers"),
			},
			{
				Keys:    bson.D{{"unpin", 1}},
				Options: options.Index().SetName("unpin"),
			},
		},
	}
}
