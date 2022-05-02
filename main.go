package main

import (
	"context"
	"log"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/api"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"gitlab.com/NebulousLabs/errors"
)

func main() {
	// Load the configuration from the environment and the local .env file.
	cfg, err := conf.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Initialise the global context and logger. These will be used throughout
	// the service. Once the context is closed, any background threads will
	// wind themselves down.
	ctx := context.Background()
	logger := logrus.New()
	logLevel, err := logrus.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Fatal(errors.AddContext(err, "invalid log level: "+cfg.LogLevel))
	}
	logger.SetLevel(logLevel)

	// Initialised the database connection.
	dbCreds := database.DBCredentials{
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
	}
	db, err := database.New(ctx, dbCreds, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, database.ErrCtxFailedToConnect))
	}

	// Initialise the server.
	server, err := api.New(cfg, db, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the api"))
	}

	err = server.ListenAndServe(4000)
	log.Fatal(err)
}
