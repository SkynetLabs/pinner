package main

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/skynetlabs/pinner/api"
	"github.com/skynetlabs/pinner/build"
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
	logger, loggerCloser, err := newLogger(cfg.LogLevel, cfg.LogFile)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to initialise logger"))
	}
	defer loggerCloser()

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

	// Initialise the server.
	server, err := api.New(cfg.ServerName, db, logger, skydClient)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the api"))
	}

	logger.Print("Starting Pinner service")
	logger.Printf("GitRevision: %v (built %v)", build.GitRevision, build.BuildTime)
	err = server.ListenAndServe(4000)
	log.Fatal(errors.Compose(err, scanner.Close()))
}

// newLogger creates a new logger that can write to disk.
//
// The function also returns a closer function that should be called when we
// stop using the logger, typically deferred in main.
func newLogger(level, logfile string) (logger *logrus.Logger, closer func(), err error) {
	logger = logrus.New()
	// Parse and set log level.
	logLevel, err := logrus.ParseLevel(level)
	if err != nil {
		return nil, nil, errors.AddContext(err, "invalid log level: "+level)
	}
	logger.SetLevel(logLevel)
	// Open and start writing to the log file, unless we have an empty string,
	// which signifies "don't log to disk".
	if logfile != "" {
		var fh *os.File
		fh, err = os.OpenFile(logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return nil, nil, errors.AddContext(err, "failed to open log file")
		}
		logger.SetOutput(io.MultiWriter(os.Stdout, fh))
		// Create a closer function which flushes the content to disk and closes
		// the log file gracefully.
		closer = func() {
			if e := fh.Sync(); e != nil {
				log.Println(errors.AddContext(err, "failed to sync log file to disk"))
				return
			}
			if e := fh.Close(); e != nil {
				log.Println(errors.AddContext(err, "failed to close log file"))
				return
			}
		}
	}
	return logger, closer, nil
}
