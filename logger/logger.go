package logger

import (
	"io"
	"os"

	"github.com/sirupsen/logrus"
	"gitlab.com/NebulousLabs/errors"
)

type (
	// ExtFieldLogger defines the logger interface we need.
	//
	// It is identical to logrus.Ext1FieldLogger but we are not using that
	// because it's marked as "Do not use". Instead, we're defining our own in
	// order to be sure that potential Logrus changes won't break us.
	ExtFieldLogger interface {
		logrus.FieldLogger
		Tracef(format string, args ...interface{})
		Trace(args ...interface{})
		Traceln(args ...interface{})
	}

	// Logger is a wrapper of *logrus.Logger which allows logging to a file on
	// disk.
	Logger struct {
		*logrus.Logger
		logFile *os.File
	}
)

// New creates a new logger that can optionally write to disk.
//
// If the given logfile argument is an empty string, the logger will not write
// to disk.
func New(level logrus.Level, logfile string) (logger *Logger, err error) {
	logger = &Logger{
		logrus.New(),
		nil,
	}
	logger.SetLevel(level)
	// Open and start writing to the log file, unless we have an empty string,
	// which signifies "don't log to disk".
	if logfile != "" {
		logger.logFile, err = os.OpenFile(logfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return nil, errors.AddContext(err, "failed to open log file")
		}

		logger.SetOutput(io.MultiWriter(os.Stdout, logger.logFile))
	}
	return logger, nil
}

// Close gracefully closes all resources used by Logger.
func (l *Logger) Close() error {
	if l.logFile == nil {
		return nil
	}
	return l.logFile.Close()
}
