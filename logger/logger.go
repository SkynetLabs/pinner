package logger

import (
	"io"
	"log"
	"os"

	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

// NewLogger creates a new logger that can write to disk.
//
// The function also returns a closer function that should be called when we
// stop using the logger, typically deferred in main.
func NewLogger(level logrus.Level, logfile string) (logger *logrus.Logger, closer func(), err error) {
	logger = logrus.New()
	logger.SetLevel(level)
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
