package logging

import (
	"context"
	"fmt"
	"os"
	"path"

	stdlog "log"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

/*
 *  Provides request and diagnostics logging facilities
 */

type ctxID int

const (
	txnIDKey ctxID = iota
)

// WithRqId returns a context which knows its transaction ID
func WithTxnID(ctx context.Context, txnID string) context.Context {
	return context.WithValue(ctx, txnIDKey, txnID)
}

type logger struct {
	logger  *logrus.Entry
	logFile *os.File
}

// The one singleton logger
var gLogger logger
var gInstanceID string

// Logger returns the global logger
func Logger(ctx context.Context) *logrus.Entry {
	if ctx != nil {
		if txnID, ok := ctx.Value(txnIDKey).(string); ok {
			return gLogger.logger.WithFields(
				logrus.Fields{
					"txnid": txnID,
				},
			)
		}
	}

	return gLogger.logger
}

func init() {
	// Viper defaults
	viper.SetDefault("logging.location", "stderr")
	viper.SetDefault("logging.format", "text")
	viper.SetDefault("logging.level", "info")

	// The app instantiation ID
	gInstanceID = uuid.New().String()

	gLogger.logger = logrus.WithFields(logrus.Fields{
		"pid":      os.Getpid(),
		"exe":      path.Base(os.Args[0]),
		"instance": gInstanceID,
	})
}

// Configure sets the log level and output location/format
func Configure(cfg *viper.Viper) error {
	// Configure system log location
	switch loc := cfg.GetString("logging.location"); loc {
	case "stdout":
		logrus.SetOutput(os.Stdout)
		gLogger.logger = logrus.WithFields(logrus.Fields{})
	case "stderr":
		logrus.SetOutput(os.Stderr)
		gLogger.logger = logrus.WithFields(logrus.Fields{})
	default:
		file, err := os.OpenFile(loc, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			gLogger.logger.Debugf("Switching system log to %s", loc)
			logrus.SetOutput(file)

			if gLogger.logFile != nil {
				gLogger.logFile.Close()
			}

			gLogger.logFile = file

			gLogger.logger = logrus.WithFields(logrus.Fields{
				"pid":      os.Getpid(),
				"exe":      path.Base(os.Args[0]),
				"instance": gInstanceID,
			})
		} else {
			return err
		}
	}

	// Obey the level setting in the config if not already in debug mode
	if !logrus.IsLevelEnabled(logrus.DebugLevel) {
		level := cfg.GetString("logging.level")
		val, err := logrus.ParseLevel(level)
		if err == nil {
			logrus.SetLevel(val)
		} else {
			return fmt.Errorf("bad log level: [%s]", level)
		}
	}

	format := cfg.GetString("logging.format")
	if format == "json" {
		logrus.SetFormatter(&logrus.JSONFormatter{})
	}

	// Override the standard system logger
	stdlog.SetOutput(Logger(nil).WriterLevel(logrus.DebugLevel))

	return nil
}
