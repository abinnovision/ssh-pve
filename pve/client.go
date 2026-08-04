// Package pve is a thin wrapper around github.com/luthermonson/go-proxmox that
// inventories every QEMU virtual machine across a whole Proxmox VE cluster and
// collects the IPv4/IPv6 addresses each VM's QEMU guest agent reports.
//
// It exists to feed a TUI that lists VMs and opens an SSH session on the
// selected one, so the returned [VM] carries exactly what that needs: a
// display name, the VMID, the hosting node, the run status, and the
// agent-reported addresses.
package pve

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/luthermonson/go-proxmox"
)

// Config holds the inputs needed to build a [Client]. APIURL, TokenID and
// TokenSecret are required; New panics if any is empty because a
// half-configured client can never succeed and failing loudly at construction
// beats a confusing 401 on first use.
type Config struct {
	// APIURL is the Proxmox API base URL including the API path, e.g.
	// "https://pve.example.com:8006/api2/json".
	APIURL string

	// TokenID is the full token identifier in the form "user@realm!tokenname".
	TokenID string

	// TokenSecret is the secret half of the API token.
	TokenSecret string

	// Insecure skips TLS certificate verification. Intended for lab clusters
	// with self-signed certs; production clusters should instead pin the CA
	// in the underlying client.
	Insecure bool

	// Concurrency caps how many VMs are queried for guest-agent network
	// interfaces at the same time. The cluster resource listing is a single
	// call, but each VM needs its own /agent/network-get-interfaces request;
	// bounding parallelism keeps a large cluster from hammering the API.
	// Defaults to 8 when zero or negative.
	Concurrency int
}

// Client is the inventory client. It wraps a go-proxmox client and exposes a
// single high-level operation, ListVMs, that fans the work out across the
// cluster.
type Client struct {
	px   *proxmox.Client
	conc int
}

const defaultConcurrency = 8

// New builds a [Client] from cfg. It does not contact the cluster; the first
// request happens when ListVMs (or any underlying call) runs.
func New(cfg Config) *Client {
	if cfg.APIURL == "" || cfg.TokenID == "" || cfg.TokenSecret == "" {
		panic("pve: New requires a non-empty APIURL, TokenID and TokenSecret")
	}

	apiURL := normalizeAPIURL(cfg.APIURL)

	opts := []proxmox.Option{
		proxmox.WithAPIToken(cfg.TokenID, cfg.TokenSecret),
	}
	if cfg.Insecure {
		opts = append(opts, proxmox.WithInsecureSkipVerify())
	}

	conc := cfg.Concurrency
	if conc <= 0 {
		conc = defaultConcurrency
	}

	return &Client{
		px:   proxmox.NewClient(apiURL, opts...),
		conc: conc,
	}
}

// normalizeAPIURL ensures the base URL carries the /api2/json path prefix that
// the PVE REST API lives under. Users frequently enter just the host root
// (e.g. "https://pve:8006") because the PVE web UI never surfaces the API
// path; without this the library builds paths like
// "https://pve:8006/cluster/status" which hit the static file handler and
// return "500 no such file" instead of the API.
func normalizeAPIURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw // let the library surface the parse error
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" || path == "/api2" || !strings.HasPrefix(path, "/api2/json") {
		u.Path = "/api2/json"
	}
	return u.String()
}

// ListVMs returns every QEMU virtual machine in the cluster together with the
// IPv4/IPv6 addresses its QEMU guest agent reports.
//
// The cluster-wide resource listing (/cluster/resources?type=vm) is fetched
// once and gives the name, VMID, node and status for every VM. Each VM is then
// queried for its guest-agent network interfaces in parallel, bounded by the
// client's Concurrency. LXC containers are filtered out — only qemu VMs are
// returned, since those are the SSH targets.
//
// A VM whose guest agent is unreachable (the VM is off, the agent is not
// installed, or the agent has not collected network info yet) is still
// returned; its IPv4/IPv6 slices stay empty and AgentError carries a short
// reason so the caller can render it distinctly. The overall error is non-nil
// only when the cluster listing itself fails.
func (c *Client) ListVMs(ctx context.Context) ([]VM, error) {
	cluster, err := c.px.Cluster(ctx)
	if err != nil {
		return nil, err
	}

	resources, err := cluster.Resources(ctx, "vm")
	if err != nil {
		if errors.Is(err, proxmox.ErrNotAuthorized) {
			return nil, fmt.Errorf("pve: token not authorized (needs Sys.Audit on / for /cluster/resources, VM.Audit + VM.GuestAgent.Audit on /vms for guest-agent IPs (VM.Monitor on PVE 8.x); with privilege separation on, assign every role to BOTH the user and the token): %w", err)
		}
		return nil, err
	}

	// Collect the qemu resources up front so the worker pool has a fixed,
	// indexable set to draw from; this also sizes the output slice.
	var indices []int
	for i, r := range resources {
		if r.Type == "qemu" {
			indices = append(indices, i)
		}
	}

	vms := make([]VM, len(indices))
	for j, i := range indices {
		r := resources[i]
		vms[j] = VM{
			Name:   r.Name,
			ID:     r.VMID,
			Node:   r.Node,
			Status: r.Status,
		}
	}

	// Fetch guest-agent IPs in parallel. Each worker pulls the next VM index
	// from the queue channel, so the work is split evenly regardless of how
	// fast individual VMs respond.
	queue := make(chan int, len(indices))
	for j := range indices {
		queue <- j
	}
	close(queue)

	workers := c.conc
	if workers > len(indices) {
		workers = len(indices)
	}

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				if err := ctx.Err(); err != nil {
					return
				}
				c.fetchAgentIPs(ctx, &vms[j])
			}
		}()
	}
	wg.Wait()

	return vms, nil
}

// fetchAgentIPs resolves a VM to a go-proxmox VirtualMachine handle and reads
// its guest-agent network interfaces, filling the VM's IPv4/IPv6 slices. On
// any failure it records a short reason in AgentError and leaves the slices
// empty so the caller never has to distinguish "no IPs" from "error" by hand.
func (c *Client) fetchAgentIPs(ctx context.Context, vm *VM) {
	node, err := c.px.Node(ctx, vm.Node)
	if err != nil {
		vm.AgentError = "node lookup: " + shortErr(err)
		return
	}

	handle, err := node.VirtualMachine(ctx, int(vm.ID)) //nolint:gosec // vm.ID is a PVE VMID, always a small integer in practice
	if err != nil {
		vm.AgentError = "vm lookup: " + shortErr(err)
		return
	}

	ifaces, err := handle.AgentGetNetworkIFaces(ctx)
	if err != nil {
		vm.AgentError = shortErr(err)
		return
	}

	for _, iface := range ifaces {
		for _, addr := range iface.IPAddresses {
			switch addr.IPAddressType {
			case "ipv4":
				vm.IPv4 = append(vm.IPv4, addr.IPAddress)
			case "ipv6":
				// Skip link-local fe80::/10 — it's never a useful SSH target
				// and only clutters the list.
				if !isLinkLocalIPv6(addr.IPAddress) {
					vm.IPv6 = append(vm.IPv6, addr.IPAddress)
				}
			}
		}
	}
}

// shortErr trims an error to its first line so AgentError stays a single,
// glanceable reason rather than a wrapped multi-line trace.
func shortErr(err error) string {
	s := err.Error()
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

// isLinkLocalIPv6 reports whether s is an IPv6 link-local address (fe80::/10).
// The guest agent reports these on every interface and they are useless as SSH
// targets, so ListVMs filters them out.
func isLinkLocalIPv6(s string) bool {
	// fe80::/10 — first byte 0xfe, second byte 0x80..0xbf.
	if len(s) < 4 || (s[0] != 'f' && s[0] != 'F') || (s[1] != 'e' && s[1] != 'E') {
		return false
	}
	// third nibble 8..b
	switch s[2] {
	case '8', '9', 'a', 'A', 'b', 'B':
		return true
	}
	return false
}
