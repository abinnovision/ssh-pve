package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigPath returns the absolute path to the config file. It honors
// $XDG_CONFIG_HOME and otherwise defaults to ~/.config/ssh-pve/config.yaml,
// matching the user's requested location on every platform.
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Dir returns the ssh-pve configuration directory. It honors $XDG_CONFIG_HOME
// and otherwise uses ~/.config/ssh-pve. Other packages (e.g. cache) use this
// to place sibling files in the same directory.
func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ssh-pve"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ssh-pve"), nil
}

// Exists reports whether a config file is present on disk. The TUI uses this
// (or errors.Is(Load's error, fs.ErrNotExist)) to decide whether to run
// onboarding on first launch.
func Exists() bool {
	p, err := ConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Load reads and parses the config file. It returns the underlying error when
// the file is missing (wrapping fs.ErrNotExist) so callers can detect first
// launch and trigger onboarding, versus a parse error which signals corruption.
func Load() (*Config, error) {
	p, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p) //nolint:gosec // path is from config dir, not user input
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save writes the config to disk, creating the parent directory (mode 0700)
// if needed. The file itself is written mode 0600 because it contains the API
// token secret and should not be world-readable.
func Save(c *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	p := filepath.Join(dir, "config.yaml")
	return os.WriteFile(p, data, 0o600)
}
