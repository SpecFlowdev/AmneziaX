// Package version carries build metadata stamped in at link time.
package version

// Version and Commit are overridden with -ldflags during release builds.
var (
	Version = "dev"
	Commit  = "unknown"
)
