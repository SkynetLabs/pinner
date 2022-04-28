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
	err := conf.LoadConf()
	if err != nil {
		log.Fatal(err)
	}

	// Initialise the global context and logger. These will be used throughout
	// the service. Once the context is closed, any background threads will
	// wind themselves down.
	ctx := context.Background()
	logger := logrus.New()
	logLevel, err := logrus.ParseLevel(conf.Conf().LogLevel)
	if err != nil {
		log.Fatal(errors.AddContext(err, "invalid log level: "+conf.Conf().LogLevel))
	}
	logger.SetLevel(logLevel)

	// Initialised the database connection.
	dbCreds := database.DBCredentials{
		User:     conf.Conf().DBUser,
		Password: conf.Conf().DBPassword,
		Host:     conf.Conf().DBHost,
		Port:     conf.Conf().DBPort,
	}
	db, err := database.New(ctx, dbCreds, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, database.ErrCtxFailedToConnect))
	}

	// Initialise the server.
	server, err := api.New(db, logger)
	if err != nil {
		log.Fatal(errors.AddContext(err, "failed to build the api"))
	}

	// TODO Start a background loop that would scan the DB for either unpinned
	// 	or underpinned skylinks and pins/unpins them.

	/*
		TODO
		 - Unpin the skylink from the local server and remove the server from the list.
		 - Keep the skylink in the DB with the unpinning flag up. This will ensure that
		 if the skylink is still pinned to any server and we sweep that sever and add
		 the skylink to the DB, it will be immediately scheduled for unpinning and it
		 will be removed from that server.
	*/

	log.Fatal(server.ListenAndServe(4000))
}
