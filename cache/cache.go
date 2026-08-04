// Package cache persists the VM inventory to disk so the TUI can display
// the last-known list instantly on launch while fetching fresh data from the
// cluster in the background.
//
// The cache file lives at ~/.config/ssh-pve/cache.yaml (or
// $XDG_CONFIG_HOME/ssh-pve/cache.yaml), beside the config file. It contains
// the VMs from the last successful fetch and a timestamp. The TUI writes it
// after every successful cluster load and reads it at startup to avoid
// making the user wait for the API before they can interact with the list.
package cache

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/abinnovision/ssh-pve/config"
	"github.com/abinnovision/ssh-pve/pve"
)

// cacheFile is the on-disk representation: a fetch timestamp plus the VMs.
type cacheFile struct {
	FetchedAt time.Time `yaml:"fetched_at"`
	VMs       []pve.VM  `yaml:"vms"`
}

// Path returns the absolute path to the cache file, in the same directory
// as the config file.
func Path() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cache.yaml"), nil
}

// Exists reports whether a cache file is present on disk.
func Exists() bool {
	p, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Load reads and parses the cache file. It returns the cached VMs (empty but
// non-nil on a valid file with no VMs) and the timestamp of the last
// successful fetch. A missing file returns the underlying *fs.PathError so
// callers can distinguish "no cache yet" from a corrupt file.
func Load() ([]pve.VM, time.Time, error) {
	p, err := Path()
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := os.ReadFile(p) //nolint:gosec // path is from config dir, not user input
	if err != nil {
		return nil, time.Time{}, err
	}
	var cf cacheFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, time.Time{}, err
	}
	return cf.VMs, cf.FetchedAt, nil
}

// Save writes the VMs to the cache file, stamping the fetch time as now.
// The parent directory is created (mode 0700) if needed. The cache file is
// mode 0600 — it holds no secrets, only cluster inventory metadata, but a
// restrictive mode is harmless and silences gosec G306.
func Save(vms []pve.VM) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cacheFile{
		FetchedAt: time.Now(),
		VMs:       vms,
	})
	if err != nil {
		return err
	}
	p, err := Path()
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}
