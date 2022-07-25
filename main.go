package main

import (
	"context"
	"log"

	"github.com/skynetlabs/pinner/api"
	"github.com/skynetlabs/pinner/build"
	"github.com/skynetlabs/pinner/conf"
	"github.com/skynetlabs/pinner/database"
	"github.com/skynetlabs/pinner/logger"
	"github.com/skynetlabs/pinner/skyd"
	"github.com/skynetlabs/pinner/sweeper"
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
	logger, err := logger.New(cfg.LogLevel, cfg.LogFile)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to initialise logger"))
	}
	defer func() {
		if err := logger.Close(); err != nil {
			log.Println(errors.AddContext(err, "failed to close logger"))
		}
	}()

	// Initialised the database connection.
	db, err := database.New(ctx, cfg.DBCredentials, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, database.ErrCtxFailedToConnect))
	}

	// Start the background scanner.
	skydClient := skyd.NewClient(cfg.SiaAPIHost, cfg.SiaAPIPort, cfg.SiaAPIPassword, skyd.NewCache(), logger)
	scanner := workers.NewScanner(db, logger, cfg.MinPinners, cfg.ServerName, cfg.SleepBetweenScans, skydClient)
	err = scanner.Start()
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to start Scanner"))
	}
	swpr := sweeper.New(db, skydClient, cfg.ServerName, logger)
	// Schedule a regular sweep..
	swpr.UpdateSchedule(sweeper.SweepInterval)

	// Initialise the server.
	server, err := api.New(cfg.ServerName, db, logger, skydClient, swpr)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the api"))
	}

	logger.Print("Starting Pinner service")
	logger.Printf("GitRevision: %v (built %v)", build.GitRevision, build.BuildTime)
	err = server.ListenAndServe(4000)
	log.Fatal(errors.Compose(err, scanner.Close()))
}
