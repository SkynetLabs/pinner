package main

import (
	"context"
	"log"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/api"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/skyd"
	"github.com/skynetlabs/pinner/workers"
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
	db, err := database.New(ctx, cfg.DBCredentials, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, database.ErrCtxFailedToConnect))
	}

	// Start the background scanner.
	skydClient := skyd.NewClient(cfg.SiaAPIHost, cfg.SiaAPIPort, cfg.SiaAPIPassword, skyd.NewCache(), logger)
	scanner := workers.NewScanner(db, logger, cfg.MinPinners, cfg.ServerName, skydClient)
	err = scanner.Start()
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to start Scanner"))
	}

	// Initialise the server.
	server, err := api.New(cfg.ServerName, db, logger, skydClient)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the api"))
	}

	err = server.ListenAndServe(4000)
	log.Fatal(errors.Compose(err, scanner.Close()))
}
