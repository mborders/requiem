package requiem

import "github.com/borderstech/logmatic"

// Logger provides application-wide logging
var Logger *logmatic.Logger

// InitLogger initializes the application-wide logger
func InitLogger(exitOnFatal bool) {
	// Create logger
	Logger = logmatic.NewLogger()
	Logger.ExitOnFatal = exitOnFatal
}
