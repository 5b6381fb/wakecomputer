package version

// Overridden at build time with -ldflags "-X .../internal/version.Version=..."
var (
	Version   = "0.1.0-dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
