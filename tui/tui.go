// Package tui implements the ssh-pve terminal UI built on bubbletea v2.
//
// The TUI has two screens:
//
//   - Onboarding: shown when no config file exists. A form collects the PVE
//     cluster endpoints, API token, default SSH user, and optional SSH command
//     template. Submitting validates the config locally, then tries a real
//     connection to the cluster — if it fails the form stays up with the error.
//     On success the config is saved to disk and the TUI transitions to the
//     VM list with the freshly-loaded VMs.
//
//   - VM list: shows every QEMU VM across the cluster with its name, ID, node
//     and run status. IP addresses are revealed only for the hovered/selected
//     row. Pressing Enter resolves the SSH command (via per-VM overrides or the
//     default template) and hands the terminal to ssh, exiting the TUI.
//
// Both screens run full-screen on the alternate screen buffer with mouse
// support (hover, click, scroll wheel) and keyboard navigation.
package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/abinnovision/ssh-pve/cache"
	"github.com/abinnovision/ssh-pve/config"
	"github.com/abinnovision/ssh-pve/pve"
)

// state tracks which screen the TUI is showing and, for the loading states,
// whether a background operation is in flight.
type state int

const (
	stateOnboarding state = iota
	stateOnboardingValidating
	stateVMListLoading
	stateVMListReady
)

// model is the single bubbletea model for the entire TUI. Screen-specific
// state lives in sub-structs (form, vms, etc.) and the Update/View methods
// dispatch on state.
type model struct {
	state   state
	width   int
	height  int
	spinner spinner.Model

	// onboarding form + config being built
	form form
	cfg  config.Config

	// vm list
	vms      []pve.VM
	selected int
	hovered  int  // -1 when no row is hovered
	scroll   int  // index of the first visible row
	fetching bool // true while a background VM load is in flight
	flash    string

	// ssh hand-off: non-empty when the user pressed Enter and Run should
	// exec the command after the TUI exits.
	sshCommand string
}

// vmsLoadedMsg is sent by the background loadVMsCmd. err is non-nil when every
// endpoint failed; vms is populated on success.
type vmsLoadedMsg struct {
	vms []pve.VM
	err error
}

// Run starts the TUI. It handles onboarding when no config file exists, then
// shows the VM list. When the user selects a VM and presses Enter, the TUI
// exits and Run execs the resolved SSH command with stdin/stdout/stderr wired
// to the terminal so ssh takes over seamlessly. Run returns when the TUI is
// quit (q/Esc) or the SSH session ends.
func Run() error {
	p := tea.NewProgram(newModel())
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	m, ok := final.(model)
	if !ok || m.sshCommand == "" {
		return nil
	}

	// The TUI has fully exited and bubbletea has restored the terminal to
	// cooked mode, so ssh inherits a clean TTY. Using "sh -c" lets the
	// template contain shell syntax (quotes, flags, etc.).
	cmd := exec.Command("sh", "-c", m.sshCommand) //nolint:gosec,noctx // sshCommand is user-controlled via config; shell interpretation is intentional; no parent context for the SSH hand-off
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// ssh exited non-zero — surface the error but don't swallow it.
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		return fmt.Errorf("tui: ssh: %w", err)
	}
	return nil
}

// newModel builds the initial model. If a config file exists it is loaded and
// the TUI starts in the VM-list state. When a cache file is also available the
// cached VMs are shown immediately and a background fetch refreshes them; if
// no cache exists the TUI shows a loading spinner until the first fetch
// completes. A corrupt config falls back to onboarding with the error shown.
func newModel() model {
	m := model{
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot)),
		hovered: -1,
	}

	if config.Exists() {
		cfg, err := config.Load()
		if err != nil {
			m.state = stateOnboarding
			m.form = newForm()
			m.form.err = "Existing config is unreadable: " + err.Error() + " — re-enter to overwrite."
			return m
		}
		m.cfg = *cfg
		m.fetching = true
		// Show cached VMs instantly if available; otherwise wait for the
		// first fetch with a loading spinner.
		if cached, _, err := cache.Load(); err == nil && len(cached) > 0 {
			m.vms = cached
			m.state = stateVMListReady
		} else {
			m.state = stateVMListLoading
		}
	} else {
		m.state = stateOnboarding
		m.form = newForm()
	}

	return m
}

// Init returns the initial commands for the current state.
func (m model) Init() tea.Cmd {
	switch m.state {
	case stateOnboarding:
		return m.form.inputs[m.form.focus].Focus()
	case stateVMListLoading:
		return tea.Batch(m.spinner.Tick, loadVMsCmd(m.cfg))
	case stateVMListReady:
		if m.fetching {
			return tea.Batch(m.spinner.Tick, loadVMsCmd(m.cfg))
		}
		return nil
	default:
		return nil
	}
}

// Update dispatches to the active screen's update handler after handling
// global messages (window size, ctrl+c, spinner ticks).
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	// Drive the spinner only while a background operation is in flight.
	if m.fetching || m.state == stateOnboardingValidating {
		if _, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	switch m.state {
	case stateOnboarding, stateOnboardingValidating:
		return m.onboardingUpdate(msg)
	case stateVMListLoading, stateVMListReady:
		return m.vmlistUpdate(msg)
	}
	return m, nil
}

// View renders the active screen. Both screens get the alt-screen buffer and
// all-mouse-motion (for hover) via the declarative tea.View fields.
func (m model) View() tea.View {
	var content string
	switch m.state {
	case stateOnboarding, stateOnboardingValidating:
		content = m.onboardingView()
	case stateVMListLoading, stateVMListReady:
		content = m.vmlistView()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

// loadVMsCmd returns a command that tries every endpoint in order and loads
// all VMs from the first one that responds. The timeout is generous because a
// large cluster can take a while to fetch guest-agent IPs for every VM.
func loadVMsCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		vms, err := loadVMs(ctx, cfg)
		return vmsLoadedMsg{vms: vms, err: err}
	}
}

// loadVMs iterates through the configured endpoints, returning the VMs from
// the first endpoint that succeeds. When all fail it returns an error wrapping
// the last failure.
func loadVMs(ctx context.Context, cfg config.Config) ([]pve.VM, error) {
	var lastErr error
	for _, ep := range cfg.Endpoints {
		client := pve.New(pve.Config{
			APIURL:      ep,
			TokenID:     cfg.APITokenID,
			TokenSecret: cfg.APITokenSecret,
			Insecure:    cfg.Insecure,
		})
		vms, err := client.ListVMs(ctx)
		if err == nil {
			return vms, nil
		}
		lastErr = fmt.Errorf("%s: %w", ep, err)
	}
	if lastErr == nil {
		return nil, fmt.Errorf("no endpoints configured")
	}
	return nil, fmt.Errorf("all endpoints failed: %w", lastErr)
}

// sshToSelected resolves the SSH command for the currently selected VM. It
// prefers IPv4, falls back to IPv6, and returns an error when the VM has no
// usable address (e.g. agent unreachable or VM stopped).
func (m model) sshToSelected() (string, error) {
	if m.selected < 0 || m.selected >= len(m.vms) {
		return "", fmt.Errorf("no VM selected")
	}
	vm := m.vms[m.selected]
	ip, err := vm.PrimaryIPv4()
	if err != nil {
		ip, err = vm.PrimaryIPv6()
		if err != nil {
			if vm.AgentError != "" {
				return "", fmt.Errorf("VM %d agent error: %s", vm.ID, vm.AgentError)
			}
			return "", fmt.Errorf("VM %d has no IP addresses", vm.ID)
		}
	}
	return m.cfg.ResolveSSH(vm.ID, ip), nil
}

// splitCSV splits a comma-separated string, trimming whitespace and dropping
// empty entries. Used to parse the endpoints field in the onboarding form.
func splitCSV(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
