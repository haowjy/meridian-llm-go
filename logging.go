package llmprovider

import "log/slog"

// globalLogger is the package-level logger for root-level functions
// that don't have access to a provider instance.
// Defaults to slog.Default() but can be overridden via SetLogger.
var globalLogger *slog.Logger = slog.Default()

// SetLogger sets the package-level logger for root-level functions.
// This should be called early in application initialization if custom logging is needed.
func SetLogger(logger *slog.Logger) {
	if logger != nil {
		globalLogger = logger
	}
}
