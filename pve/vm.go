package pve

import "errors"

// VM is one QEMU virtual machine in the cluster as seen by the inventory
// client. The identity fields (Name, ID, Node, Status) always come from the
// cluster resource listing and are populated for every VM. The address slices
// are filled from the QEMU guest agent; they stay empty when the agent is
// unreachable, in which case AgentError explains why.
type VM struct {
	// Name is the VM's name as configured in Proxmox. May be empty for VMs
	// that were never given one.
	Name string `yaml:"name"`

	// ID is the Proxmox VMID (e.g. 100).
	ID uint64 `yaml:"id"`

	// Node is the cluster node the VM currently runs on.
	Node string `yaml:"node"`

	// Status is the VM run state as reported by the cluster resource listing
	// (e.g. "running", "stopped").
	Status string `yaml:"status"`

	// IPv4 holds the global IPv4 addresses the guest agent reported, across
	// all non-loopback interfaces. Empty when the agent is unreachable or has
	// not reported any IPv4 address.
	IPv4 []string `yaml:"ipv4,omitempty"`

	// IPv6 holds the global (non-link-local) IPv6 addresses the guest agent
	// reported. Link-local fe80::/10 addresses are filtered out because they
	// are never usable SSH targets.
	IPv6 []string `yaml:"ipv6,omitempty"`

	// AgentError is empty when the guest-agent network query succeeded. When
	// non-empty it holds a short, single-line reason the agent could not be
	// queried (VM off, agent not installed, permission denied, ...), suitable
	// for rendering next to the VM in a list.
	AgentError string `yaml:"agent_error,omitempty"`
}

// ErrNoIP is the sentinel returned by [VM.PrimaryIPv4] and [VM.PrimaryIPv6]
// when the VM has no address of the requested family. Callers can tell "no IP
// for this VM" apart from a genuine error via errors.Is.
var ErrNoIP = errors.New("pve: no agent-reported IP address")

// PrimaryIPv4 returns the first IPv4 address the guest agent reported, or the
// empty string with error ErrNoIP when there is none. A TUI that SSHes on Enter
// typically wants exactly this.
func (v VM) PrimaryIPv4() (string, error) {
	if len(v.IPv4) == 0 {
		return "", ErrNoIP
	}
	return v.IPv4[0], nil
}

// PrimaryIPv6 returns the first non-link-local IPv6 address the guest agent
// reported, or "" with error ErrNoIP when there is none.
func (v VM) PrimaryIPv6() (string, error) {
	if len(v.IPv6) == 0 {
		return "", ErrNoIP
	}
	return v.IPv6[0], nil
}

// Running reports whether the VM is currently running, i.e. Status == "running".
// Convenience for TUIs that want to grey out or skip stopped VMs.
func (v VM) Running() bool {
	return v.Status == "running"
}
