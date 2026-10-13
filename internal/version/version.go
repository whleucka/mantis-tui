// Package version reports which mantis-tui this is.
package version

import "runtime/debug"

// stamped is set at build time by the Makefile:
//
//	-ldflags "-X github.com/whleucka/mantis-tui/internal/version.stamped=v0.0.2"
var stamped string

// String is the stamped version, else the module version `go install`
// records, else "dev".
func String() string {
	if stamped != "" {
		return stamped
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
