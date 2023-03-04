// Package log contains the logging functions used by protosync.
package log

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/colour"
	"github.com/pkg/errors"
)

//go:generate stringer -linecomment -type Level

// Config for logger.
type Config struct {
	Level Level `help:"Minimum log level." default:"info"`
}

// Configure global logging.
func Configure(config Config) error {
	MinLevel = config.Level
	return nil
}

// Level for a log message.
type Level int

// Log levels.
const (
	LevelTrace Level = iota // trace
	LevelDebug              // debug
	LevelInfo               // info
	LevelWarn               // warn
	LevelError              // error
	LevelFatal              // fatal
)

func (l *Level) UnmarshalText(text []byte) error { //nolint:golint
	var err error
	*l, err = LevelFromString(string(text))
	return err
}

// LevelFromString maps a level to a string.
func LevelFromString(s string) (Level, error) {
	switch s {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn", "warning":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "fatal":
		return LevelFatal, nil
	default:
		return 0, errors.Errorf("invalid log level %q", s)
	}
}

// WriterFlusher is used to flush log output after each line.
type WriterFlusher interface {
	io.Writer
	Sync() error
}

var (
	// Levels of logging.
	Levels = []Level{LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError}
	// MinLevel of displayed logs.
	MinLevel = LevelWarn
	// LogOutput is the stdout for logs.
	LogOutput WriterFlusher = os.Stdout
	// LogError is the stderr for logs and where error+fatal logs are sent.
	LogError WriterFlusher = os.Stderr
	// Root logger.
	Root = &Logger{}

	levelColor = map[Level]string{
		LevelTrace: "",
		LevelDebug: "^6",
		LevelInfo:  "^2",
		LevelWarn:  "^3",
		LevelError: "^3",
		LevelFatal: "^3",
	}
)

// Logger is a scoped logging object.
type Logger struct {
	prefix []string
	buf    []byte
}

// Debugf logs a debug message.
func Debugf(format string, args ...interface{}) { Root.Debugf(format, args...) }

// Tracef logs a trace message.
func Tracef(format string, args ...interface{}) { Root.Tracef(format, args...) }

// Infof logs an informational message.
func Infof(format string, args ...interface{}) { Root.Infof(format, args...) }

// Warnf logs a warning.
func Warnf(format string, args ...interface{}) { Root.Warnf(format, args...) }

// Errorf logs an error.
func Errorf(format string, args ...interface{}) { Root.Errorf(format, args...) }

// Fatalf logs an error and terminates the process.
func Fatalf(format string, args ...interface{}) { Root.Logf(LevelFatal, format, args...) }

// Logf logs at the given level.
func Logf(level Level, format string, args ...interface{}) { Root.Logf(level, format, args...) }

// Debugf logs a debug message.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Logf(LevelDebug, format, args...)
}

// Tracef logs a trace message.
func (l *Logger) Tracef(format string, args ...interface{}) {
	l.Logf(LevelTrace, format, args...)
}

// Infof logs an informational message.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Logf(LevelInfo, format, args...)
}

// Warnf logs a warning.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Logf(LevelWarn, format, args...)
}

// Errorf logs an error.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Logf(LevelError, format, args...)
}

// Fatalf logs an error and terminates the process.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.Logf(LevelFatal, format, args...)
	os.Exit(1)
}

// Logf logs at the given level.
func (l *Logger) Logf(level Level, format string, args ...interface{}) {
	if level < MinLevel {
		return
	}
	out := LogOutput
	if level == LevelError {
		out = LogError
	} else if level == LevelFatal {
		out = os.Stderr
	}
	format = fmt.Sprintf("^B%s%s:%s^R%s%s^R\n", levelColor[level], level, l.prefixIt(),
		levelColor[level], format)
	_, _ = colour.Colour(out).Printf(format, args...)
	_ = out.Sync()
}

// SubLogger creates a new sub-logger from a string prefix (or Builder).
func (l *Logger) SubLogger(id string) *Logger {
	return &Logger{prefix: append(l.prefix, id)}
}

// Write to the logger. Each line will have the logger prefix prepended.
func (l *Logger) Write(b []byte) (int, error) {
	l.buf = append(l.buf, b...)
	for i := bytes.IndexByte(l.buf, '\n'); i != -1; i = bytes.IndexByte(l.buf, '\n') {
		l.Debugf("%s", l.buf[:i])
		if i >= len(l.buf) {
			l.buf = nil
			break
		}
		l.buf = l.buf[i+1:]
	}
	return len(b), nil
}

func (l *Logger) prefixIt() string {
	if len(l.prefix) == 0 {
		return " "
	}
	return strings.Join(l.prefix, ":") + ": "
}

// Elapsed logs the duration of a function call. Use with defer:
//
//	defer Elapsed(log, "something")()
func Elapsed(log *Logger, message string, args ...interface{}) func() {
	start := time.Now()
	return func() {
		args = append(args, time.Since(start))
		log.Tracef(message+" (%s elapsed)", args...)
	}
}
