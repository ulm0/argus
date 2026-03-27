package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// L is the application-wide logger. All packages should import and use this
// instead of the stdlib log package.
var L = logrus.New()

func init() {
	L.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02T15:04:05Z07:00",
		DisableColors:   true,
	})
	L.SetOutput(os.Stdout)
	L.SetLevel(logrus.DebugLevel)
}

// SetLevel updates the global log level at runtime.
func SetLevel(level logrus.Level) {
	L.SetLevel(level)
}

// SetLevelFromString parses a human-readable level name and applies it.
// Accepted values: trace, debug, info, warn/warning, error, fatal, panic.
// Returns false if the string is unrecognised (level is left unchanged).
func SetLevelFromString(s string) bool {
	lvl, err := logrus.ParseLevel(strings.TrimSpace(strings.ToLower(s)))
	if err != nil {
		return false
	}
	L.SetLevel(lvl)
	return true
}

// LevelString returns the current log level as a lowercase string.
func LevelString() string {
	return L.GetLevel().String()
}
