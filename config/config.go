// Package config manages the ssh-pve TUI's on-disk configuration store.
//
// The config file lives at ~/.config/ssh-pve/config.yaml (or under
// $XDG_CONFIG_HOME/ssh-pve/ when that env var is set) and holds everything the
// TUI needs to reach a Proxmox VE cluster and open SSH sessions on its VMs:
//
//   - the ordered list of cluster API endpoints (tried in turn so a down node
//     falls through to the next),
//   - the API token used to authenticate against the cluster,
//   - the default SSH user,
//   - an optional SSH command template with {{user}} and {{ip}} placeholders,
//   - per-VM overrides that change the user and/or command template for a
//     single VMID.
//
// Onboarding (a TUI concern, implemented later) calls [Default] to seed a
// config, prompts the user, and saves it with [Save]. Subsequent launches call
// [Load] and branch into onboarding only when the file is missing.
package config

import (
	"errors"
	"net/url"
	"strings"
)

// defaultTemplate is the SSH command used when no template is configured at
// any level of the override chain. Exported as a constant so the onboarding
// flow can offer it as the suggested default.
const defaultTemplate = "ssh {{user}}@{{ip}}"

// Config is the top-level on-disk configuration.
type Config struct {
	// Endpoints is the ordered list of PVE API URLs (scheme + host + port,
	// e.g. "https://pve1.example.com:8006"). The TUI tries them in order so
	// the first reachable node wins; a down node falls through to the next.
	// At least one entry is required.
	Endpoints []string `yaml:"endpoints"`

	// APITokenID is the PVE API token identifier in the form
	// "user@realm!tokenname", e.g. "inventory@pve!readonly".
	APITokenID string `yaml:"api_token_id"`

	// APITokenSecret is the secret half of the API token. Stored in
	// plaintext on disk; the file is written mode 0600 by [Save].
	APITokenSecret string `yaml:"api_token_secret"`

	// Insecure disables TLS certificate verification when talking to the
	// PVE API. Intended for lab clusters using self-signed certificates;
	// leave false in production. The onboarding form exposes this as a
	// "Verify TLS" checkbox (unchecked → Insecure true).
	Insecure bool `yaml:"insecure,omitempty"`

	// DefaultSSHUser is the SSH user used for every VM that has no
	// per-VM override. Required.
	DefaultSSHUser string `yaml:"default_ssh_user"`

	// SSHCommandTemplate is the optional command template applied to every
	// connection unless a [VMOverride] supplies its own. The placeholders
	// {{user}} and {{ip}} are substituted at render time by [ResolveSSH].
	// When empty, the built-in default "ssh {{user}}@{{ip}}" is used.
	SSHCommandTemplate string `yaml:"ssh_command_template,omitempty"`

	// VMOverrides customizes connection behavior for specific VMs keyed by
	// VMID. A VM with no entry uses the top-level defaults.
	VMOverrides map[uint64]VMOverride `yaml:"vm_overrides,omitempty"`
}

// VMOverride customizes the SSH connection for a single VM. Every field is
// optional; an empty field means "inherit the top-level default".
type VMOverride struct {
	// SSHUser overrides [Config.DefaultSSHUser] for this VM.
	SSHUser string `yaml:"ssh_user,omitempty"`

	// SSHCommandTemplate overrides [Config.SSHCommandTemplate] for this VM.
	SSHCommandTemplate string `yaml:"ssh_command_template,omitempty"`
}

// Default returns a starter [Config] with placeholder values that the
// onboarding flow is expected to populate before saving. Endpoints and the
// token fields are intentionally blank-ish so the caller fills them in; the
// SSH user and command template get working defaults so the config is
// functional the moment the cluster bits are filled.
func Default() *Config {
	return &Config{
		Endpoints:          []string{"https://your-pve-host:8006/api2/json"},
		DefaultSSHUser:     "root",
		SSHCommandTemplate: defaultTemplate,
	}
}

// ErrInvalidConfig is returned by [Config.Validate] when required fields are
// missing or malformed. The TUI can use it to decide whether to (re)run
// onboarding.
var ErrInvalidConfig = errors.New("config: invalid configuration")

// Validate reports whether the config has the minimum required fields
// populated and well-formed. It checks that endpoints parse as URLs with a
// scheme and host, that the token ID and secret are non-empty, and that a
// default SSH user is set. It does not contact the cluster.
func (c *Config) Validate() error {
	if len(c.Endpoints) == 0 {
		return ErrInvalidConfig
	}
	for _, ep := range c.Endpoints {
		u, err := url.Parse(ep)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return ErrInvalidConfig
		}
	}
	if c.APITokenID == "" || c.APITokenSecret == "" {
		return ErrInvalidConfig
	}
	if c.DefaultSSHUser == "" {
		return ErrInvalidConfig
	}
	return nil
}

// ResolveSSH returns the fully-rendered SSH command for connecting to the
// given IP on the VM identified by vmid.
//
// Resolution applies per-VM overrides on top of the top-level defaults: a
// [VMOverride] that sets SSHUser wins over [Config.DefaultSSHUser], and one
// that sets SSHCommandTemplate wins over [Config.SSHCommandTemplate]. An empty
// template at any level falls back to the built-in default
// "ssh {{user}}@{{ip}}". The returned string is ready to pass to exec.
func (c *Config) ResolveSSH(vmid uint64, ip string) string {
	user := c.DefaultSSHUser
	tmpl := c.SSHCommandTemplate
	if ov, ok := c.VMOverrides[vmid]; ok {
		if ov.SSHUser != "" {
			user = ov.SSHUser
		}
		if ov.SSHCommandTemplate != "" {
			tmpl = ov.SSHCommandTemplate
		}
	}
	if tmpl == "" {
		tmpl = defaultTemplate
	}
	return renderTemplate(tmpl, user, ip)
}

// renderTemplate substitutes the {{user}} and {{ip}} placeholders in tmpl.
// A simple replacer is used instead of text/template so the template syntax
// stays predictable and free of injection surface.
func renderTemplate(tmpl, user, ip string) string {
	return strings.NewReplacer("{{user}}", user, "{{ip}}", ip).Replace(tmpl)
}
