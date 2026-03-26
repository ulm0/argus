package logger

import (
	"os"

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
	L.SetLevel(logrus.InfoLevel)
}

// SetLevel updates the global log level at runtime (e.g. from a --debug flag).
func SetLevel(level logrus.Level) {
	L.SetLevel(level)
}
