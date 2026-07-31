package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Column styles for the VM list. Each uses a fixed Width so the columns line
// up regardless of content length; lipgloss accounts for ANSI codes when
// calculating rendered width.
var (
	colID            = lipgloss.NewStyle().Width(6)
	colName          = lipgloss.NewStyle().Width(20)
	colNode          = lipgloss.NewStyle().Width(10)
	colStatusRunning = lipgloss.NewStyle().Width(10).Foreground(colorSuccess)
	colStatusStopped = lipgloss.NewStyle().Width(10).Foreground(colorMuted)
)

// listStartY is the absolute terminal row where the first VM row is rendered.
// It is the sum of the frame's top border (1), the title line (1), a blank
// separator (1), and the column-header line (1).
const listStartY = 4

// listHeight returns how many VM rows fit in the current terminal. The
// non-list rows are: 2 frame borders, title, blank, column headers, blank,
// hints — 7 rows total.
func (m model) listHeight() int {
	h := m.height
	if h == 0 {
		h = 24
	}
	lh := h - 7
	if lh < 1 {
		lh = 1
	}
	return lh
}

// adjustScroll keeps the selected row inside the visible window after the
// selection or viewport changes.
func (m *model) adjustScroll() {
	lh := m.listHeight()
	if m.selected < m.scroll {
		m.scroll = m.selected
	}
	if m.selected >= m.scroll+lh {
		m.scroll = m.selected - lh + 1
	}
	maxScroll := len(m.vms) - lh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.scroll > maxScroll {
		m.scroll = maxScroll
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// vmlistUpdate dispatches to the loading or ready sub-handler.
func (m model) vmlistUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateVMListLoading:
		return m.vmlistLoadingUpdate(msg)
	case stateVMListReady:
		return m.vmlistReadyUpdate(msg)
	}
	return m, nil
}

// vmlistLoadingUpdate waits for VMs to load and allows the user to quit.
func (m model) vmlistLoadingUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case vmsLoadedMsg:
		m.state = stateVMListReady
		if msg.err != nil {
			m.flash = "Failed to load VMs: " + msg.err.Error()
			return m, nil
		}
		m.vms = msg.vms
		m.selected = 0
		m.hovered = -1
		m.scroll = 0
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "esc" || msg.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// vmlistReadyUpdate handles keyboard navigation, mouse hover/click/scroll, and
// the Enter-to-SSH hand-off.
func (m model) vmlistReadyUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
				m.adjustScroll()
				m.flash = ""
			}
		case "down", "j":
			if m.selected < len(m.vms)-1 {
				m.selected++
				m.adjustScroll()
				m.flash = ""
			}
		case "enter":
			cmd, err := m.sshToSelected()
			if err != nil {
				m.flash = err.Error()
				return m, nil
			}
			m.sshCommand = cmd
			return m, tea.Quit
		case "q", "esc":
			return m, tea.Quit
		}

	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		idx := mouse.Y - listStartY + m.scroll
		if idx >= 0 && idx < len(m.vms) {
			m.hovered = idx
		} else {
			m.hovered = -1
		}

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			mouse := msg.Mouse()
			idx := mouse.Y - listStartY + m.scroll
			if idx >= 0 && idx < len(m.vms) {
				m.selected = idx
				m.adjustScroll()
				m.flash = ""
			}
		}

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			if m.scroll > 0 {
				m.scroll--
			}
		case tea.MouseWheelDown:
			maxScroll := len(m.vms) - m.listHeight()
			if maxScroll < 0 {
				maxScroll = 0
			}
			if m.scroll < maxScroll {
				m.scroll++
			}
		}
	}

	return m, nil
}

// vmlistView renders the loading spinner or the full VM list.
func (m model) vmlistView() string {
	w := m.width
	if w == 0 {
		w = 80
	}
	h := m.height
	if h == 0 {
		h = 24
	}

	var content string
	if m.state == stateVMListLoading {
		content = m.spinner.View() + "  Loading VMs..."
	} else {
		content = m.readyView()
	}

	return styleFrame.Width(w).Height(h).Render(content)
}

// readyView renders the full VM list with title, column headers, rows, and a
// hint/flash bar at the bottom.
func (m model) readyView() string {
	var b strings.Builder

	// Title
	b.WriteString(styleTitle.Render(fmt.Sprintf("ssh-pve — VM List (%d VMs)", len(m.vms))))
	b.WriteString("\n\n")

	// Column headers
	header := fmt.Sprintf("  %-6s  %-20s  %-10s  %-10s", "ID", "Name", "Node", "Status")
	b.WriteString(styleDesc.Render(header))
	b.WriteString("\n")

	// VM rows or empty-state message
	lh := m.listHeight()
	if len(m.vms) == 0 {
		if m.flash != "" {
			b.WriteString(styleError.Render(m.flash))
		} else {
			b.WriteString(styleMuted.Render("No VMs found in cluster."))
		}
		b.WriteString("\n")
		for j := 1; j < lh; j++ {
			b.WriteString("\n")
		}
	} else {
		end := m.scroll + lh
		if end > len(m.vms) {
			end = len(m.vms)
		}
		for i := m.scroll; i < end; i++ {
			b.WriteString(m.renderVMRow(i))
			b.WriteString("\n")
		}
		// Pad remaining list area with blank lines so the hint bar stays
		// at the bottom.
		for j := end - m.scroll; j < lh; j++ {
			b.WriteString("\n")
		}
	}

	// Separator + hint/flash bar
	b.WriteString("\n")
	if m.flash != "" && len(m.vms) > 0 {
		b.WriteString(styleError.Render(m.flash))
	} else {
		b.WriteString(styleHint.Render("↑↓/jk: navigate  Enter: SSH  q: quit  hover/click: select"))
	}

	return b.String()
}

// renderVMRow renders a single VM as one line. The selected or hovered row
// additionally reveals the guest-agent IP addresses (or an agent error).
func (m model) renderVMRow(i int) string {
	vm := m.vms[i]
	isSel := i == m.selected
	isHover := i == m.hovered && !isSel

	marker := " "
	if isSel {
		marker = "▸"
	}

	idCol := colID.Render(fmt.Sprintf("%d", vm.ID))
	nameCol := colName.Render(truncate(vm.Name, 20))
	nodeCol := colNode.Render(truncate(vm.Node, 10))

	var statusCol string
	if vm.Running() {
		statusCol = colStatusRunning.Render(vm.Status)
	} else {
		statusCol = colStatusStopped.Render(vm.Status)
	}

	row := fmt.Sprintf("%s %s  %s  %s  %s", marker, idCol, nameCol, nodeCol, statusCol)

	// Reveal IPs (or agent error) only for the selected or hovered row.
	if isSel || isHover {
		const sep = "  "
		w := m.width
		if w == 0 {
			w = 80
		}
		avail := w - 6 - lipgloss.Width(row) - len(sep)
		if avail < 1 {
			avail = 1
		}
		switch {
		case len(vm.IPv4) > 0:
			row += sep + styleIP.Render(ansi.Truncate(strings.Join(vm.IPv4, "  "), avail, "…"))
		case len(vm.IPv6) > 0:
			row += sep + styleIP.Render(ansi.Truncate(strings.Join(vm.IPv6, "  "), avail, "…"))
		case vm.AgentError != "":
			row += sep + styleError.Render("(" + vm.AgentError + ")")
		default:
			row += sep + styleMuted.Render("(no IPs)")
		}
	}

	return row
}

// truncate shortens s to n runes, appending an ellipsis when truncated.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
