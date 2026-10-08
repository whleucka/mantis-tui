package config

import "path/filepath"

// DefaultFilesDir is where downloaded attachments are cached:
// $XDG_CACHE_HOME/mantis-tui/files, falling back to ~/.cache.
func DefaultFilesDir(getenv func(string) string) string {
	base := getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(getenv("HOME"), ".cache")
	}
	return filepath.Join(base, "mantis-tui", "files")
}
