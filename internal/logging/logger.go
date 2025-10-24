package logging

import (
	"fmt"
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

type structuredLogger struct {
	base *log.Logger
}

func (l *structuredLogger) Debug(args ...interface{}) { l.log(log.DebugLevel, args...) }
func (l *structuredLogger) Debugf(format string, args ...interface{}) {
	l.base.Debugf(format, args...)
}

func (l *structuredLogger) Info(args ...interface{}) { l.log(log.InfoLevel, args...) }
func (l *structuredLogger) Infof(format string, args ...interface{}) {
	l.base.Infof(format, args...)
}

func (l *structuredLogger) Warn(args ...interface{}) { l.log(log.WarnLevel, args...) }
func (l *structuredLogger) Warnf(format string, args ...interface{}) {
	l.base.Warnf(format, args...)
}

func (l *structuredLogger) Error(args ...interface{}) { l.log(log.ErrorLevel, args...) }
func (l *structuredLogger) Errorf(format string, args ...interface{}) {
	l.base.Errorf(format, args...)
}

func (l *structuredLogger) Fatal(args ...interface{}) { l.log(log.FatalLevel, args...) }
func (l *structuredLogger) Fatalf(format string, args ...interface{}) {
	l.base.Fatalf(format, args...)
}

func (l *structuredLogger) log(level log.Level, args ...interface{}) {
	if l.base == nil {
		return
	}

	msg, fields := parseArgs(args...)
	entry := log.NewEntry(l.base)
	if len(fields) > 0 {
		entry = entry.WithFields(fields)
	}
	entry.Log(level, msg)
}

func parseArgs(args ...interface{}) (string, log.Fields) {
	if len(args) == 0 {
		return "", nil
	}

	msg := fmt.Sprint(args[0])
	if len(args) == 1 {
		return msg, nil
	}

	fields := log.Fields{}
	for i := 1; i < len(args); i++ {
		key, ok := args[i].(string)
		if ok {
			if i+1 < len(args) {
				fields[key] = args[i+1]
				i++
			} else {
				fields[key] = "(missing value)"
			}
			continue
		}

		if err, isErr := args[i].(error); isErr {
			fields["error"] = err
			continue
		}

		fields[fmt.Sprintf("arg_%d", i)] = args[i]
	}

	return msg, fields
}

var defaultLogger Logger = &structuredLogger{base: log.StandardLogger()}

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

func L() Logger { return defaultLogger }

// NewDefaultLogger creates a default logger
func NewDefaultLogger() Logger {
	return &structuredLogger{base: log.StandardLogger()}
}
