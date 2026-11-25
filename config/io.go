package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

// ConfigPath returns the absolute path to the config file, config.yaml inside
// [Dir].
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Dir returns the ssh-pve configuration directory. It honors $XDG_CONFIG_HOME
// on every platform; otherwise it uses %AppData%\ssh-pve on Windows and
// ~/.config/ssh-pve elsewhere. Other packages (e.g. cache) use this to place
// sibling files in the same directory.
func Dir() (string, error) {
	return dirFor(runtime.GOOS, os.Getenv, os.UserHomeDir)
}

// dirFor implements [Dir] with the OS, environment and home lookup injected so
// the per-platform rules can be tested on any host.
func dirFor(goos string, getenv func(string) string, homeDir func() (string, error)) (string, error) {
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ssh-pve"), nil
	}
	if goos == "windows" {
		appData := getenv("APPDATA")
		if appData == "" {
			return "", errors.New("%AppData% is not defined")
		}
		return filepath.Join(appData, "ssh-pve"), nil
	}
	home, err := homeDir()
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
// token secret and should not be world-readable. Windows ignores these modes;
// there the file is protected only by the ACLs of the user's profile.
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
