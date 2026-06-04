// Package log provides a small leveled logger used across the SDK,
// mirroring aip_sdk/utils/logger.py.
package log

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
)

// Level controls logging verbosity.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarning
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarning:
		return "WARNING"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

var (
	mu       sync.RWMutex
	minLevel = LevelInfo
	loggers  = map[string]*Logger{}
)

// Logger is a named, leveled logger.
type Logger struct {
	name string
	std  *log.Logger
}

// Get returns (or creates) a logger for the given name, mirroring the
// lru_cache behavior of the Python get_logger helper.
func Get(name string) *Logger {
	mu.Lock()
	defer mu.Unlock()
	if l, ok := loggers[name]; ok {
		return l
	}
	l := &Logger{
		name: "unibase." + name,
		std:  log.New(os.Stdout, "", log.LstdFlags),
	}
	loggers[name] = l
	return l
}

// SetLevel sets the global minimum log level.
func SetLevel(level Level) {
	mu.Lock()
	minLevel = level
	mu.Unlock()
}

// SetLevelFromEnv reads UNIBASE_LOG_LEVEL and applies it if set.
func SetLevelFromEnv() {
	switch strings.ToUpper(os.Getenv("UNIBASE_LOG_LEVEL")) {
	case "DEBUG":
		SetLevel(LevelDebug)
	case "INFO":
		SetLevel(LevelInfo)
	case "WARNING", "WARN":
		SetLevel(LevelWarning)
	case "ERROR":
		SetLevel(LevelError)
	}
}

func enabled(level Level) bool {
	mu.RLock()
	defer mu.RUnlock()
	return level >= minLevel
}

func (l *Logger) emit(level Level, format string, args ...any) {
	if !enabled(level) {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	l.std.Printf("%s - %s - %s", l.name, level, msg)
}

// Debugf logs at debug level.
func (l *Logger) Debugf(format string, args ...any) { l.emit(LevelDebug, format, args...) }

// Infof logs at info level.
func (l *Logger) Infof(format string, args ...any) { l.emit(LevelInfo, format, args...) }

// Warnf logs at warning level.
func (l *Logger) Warnf(format string, args ...any) { l.emit(LevelWarning, format, args...) }

// Errorf logs at error level.
func (l *Logger) Errorf(format string, args ...any) { l.emit(LevelError, format, args...) }
