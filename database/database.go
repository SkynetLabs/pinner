package database

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var (
	// dbName defines the name of the database this service uses
	dbName = "pinner"
	// collSkylinks defines the name of the collection which will hold
	// information about skylinks
	collSkylinks = "skylinks"

	// mongoCompressors defines the compressors we are going to use for the
	// connection to MongoDB
	mongoCompressors = "zstd,zlib,snappy"
	// mongoReadPreference defines the DB's read preference. The options are:
	// primary, primaryPreferred, secondary, secondaryPreferred, nearest.
	// See https://docs.mongodb.com/manual/core/read-preference/
	mongoReadPreference = "primary"
	// mongoWriteConcern describes the level of acknowledgment requested from
	// MongoDB.
	mongoWriteConcern = "majority"
	// mongoWriteConcernTimeout specifies a time limit, in milliseconds, for
	// the write concern to be satisfied.
	mongoWriteConcernTimeout = "30000"
)

type (
	// DB holds a connection to the database, as well as helpful shortcuts to
	// collections and utilities.
	DB struct {
		staticCtx    context.Context
		staticDB     *mongo.Database
		staticLogger *logrus.Logger
	}

	// DBCredentials is a helper struct that binds together all values needed for
	// establishing a DB connection.
	DBCredentials struct {
		User     string
		Password string
		Host     string
		Port     string
	}
)

// New creates a new database connection.
func New(ctx context.Context, creds DBCredentials, logger *logrus.Logger) (*DB, error) {
	return NewCustomDB(ctx, dbName, creds, logger)
}

// NewCustomDB creates a new database connection to a database with a custom name.
func NewCustomDB(ctx context.Context, dbName string, creds DBCredentials, logger *logrus.Logger) (*DB, error) {
	if ctx == nil {
		return nil, errors.New("invalid context provided")
	}
	if logger == nil {
		return nil, errors.New("invalid logger provided")
	}
	c, err := mongo.NewClient(options.Client().ApplyURI(connectionString(creds)))
	if err != nil {
		return nil, errors.AddContext(err, "failed to create a new db client")
	}
	err = c.Connect(ctx)
	if err != nil {
		return nil, errors.AddContext(err, "failed to connect to db")
	}
	db := c.Database(dbName)
	err = ensureDBSchema(ctx, db, logger)
	if err != nil {
		return nil, err
	}
	return &DB{
		staticCtx:    ctx,
		staticDB:     db,
		staticLogger: logger,
	}, nil
}

// Disconnect closes the connection to the database in an orderly fashion.
func (db *DB) Disconnect(ctx context.Context) error {
	return db.staticDB.Client().Disconnect(ctx)
}

// Ping sends a ping command to verify that the client can connect to the DB and
// specifically to the primary.
func (db *DB) Ping(ctx context.Context) error {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.staticDB.Client().Ping(ctx2, readpref.Primary())
}

// connectionString is a helper that returns a valid MongoDB connection string
// based on the passed credentials and a set of constants. The connection string
// is using the standalone approach because the service is supposed to talk to
// the replica set only via the local node.
// See https://docs.mongodb.com/manual/reference/connection-string/
func connectionString(creds DBCredentials) string {
	// There are some symbols in usernames and passwords that need to be escaped.
	// See https://docs.mongodb.com/manual/reference/connection-string/#components
	return fmt.Sprintf(
		"mongodb://%s:%s@%s:%s/?compressors=%s&readPreference=%s&w=%s&wtimeoutMS=%s",
		url.QueryEscape(creds.User),
		url.QueryEscape(creds.Password),
		creds.Host,
		creds.Port,
		mongoCompressors,
		mongoReadPreference,
		mongoWriteConcern,
		mongoWriteConcernTimeout,
	)
}

// ensureDBSchema checks that we have all collections and indexes we need and
// creates them if needed.
// See https://docs.mongodb.com/manual/indexes/
// See https://docs.mongodb.com/manual/core/index-unique/
func ensureDBSchema(ctx context.Context, db *mongo.Database, log *logrus.Logger) error {
	for collName, models := range schema {
		coll, err := ensureCollection(ctx, db, collName)
		if err != nil {
			return err
		}
		iv := coll.Indexes()
		var names []string
		names, err = iv.CreateMany(ctx, models)
		if err != nil {
			return errors.AddContext(err, "failed to create indexes")
		}
		log.Debugf("Ensured index exists: %v", names)
	}
	return nil
}

// ensureCollection gets the given collection from the
// database and creates it if it doesn't exist.
func ensureCollection(ctx context.Context, db *mongo.Database, collName string) (*mongo.Collection, error) {
	coll := db.Collection(collName)
	if coll == nil {
		err := db.CreateCollection(ctx, collName)
		if err != nil {
			return nil, err
		}
		coll = db.Collection(collName)
		if coll == nil {
			return nil, errors.New("failed to create collection " + collName)
		}
	}
	return coll, nil
}
