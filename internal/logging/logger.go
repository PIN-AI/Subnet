package logging

import (
	"os"

	log "github.com/sirupsen/logrus"
)

// Logger is the interface for logging
type Logger interface {
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
}

func Init(level string) {
	log.SetOutput(os.Stdout)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})
	// default info
	l, err := log.ParseLevel(level)
	if err != nil {
		l = log.InfoLevel
	}
	log.SetLevel(l)
}

func L() *log.Logger { return log.StandardLogger() }

// NewDefaultLogger creates a default logger
func NewDefaultLogger() Logger {
	return log.StandardLogger()
}
