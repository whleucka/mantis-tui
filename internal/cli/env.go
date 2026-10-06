package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/whleucka/mantis-tui/internal/config"
)

// loadConfig reads --config (or the default path), falling back to
// auto-detected MANTIS_* env pairs, and resolves every host's token. Config
// warnings are written to stderr.
func (o *globalOpts) loadConfig(stderr io.Writer) (*config.Config, config.Resolution, error) {
	path, explicit := o.configPath, o.configPath != ""
	if !explicit {
		path = config.DefaultConfigPath(os.Getenv)
	}

	cfg, err := config.Load(path, explicit)
	if errors.Is(err, config.ErrNoConfig) {
		cfg = config.AutoDetect(os.Environ())
		if cfg == nil {
			return nil, config.Resolution{}, fmt.Errorf(
				"no config at %s and no MANTIS_<NAME> + MANTIS_<NAME>_URL env pairs found; create one like:\n\n%s",
				path, config.SampleConfig)
		}
	} else if err != nil {
		return nil, config.Resolution{}, err
	}

	for _, w := range cfg.Warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	return cfg, config.Resolve(cfg, os.Environ()), nil
}
