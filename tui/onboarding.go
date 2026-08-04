package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/abinnovision/ssh-pve/cache"
	"github.com/abinnovision/ssh-pve/config"
)

// Field indices in the onboarding form. Indices 0..fieldSSHTemplate are
// textinputs backed by form.inputs; fieldVerifyTLS is a boolean toggle with
// no textinput. fieldCount is the total number of focusable fields.
const (
	fieldEndpoints = iota
	fieldTokenID
	fieldTokenSecret
	fieldSSHUser
	fieldSSHTemplate
	fieldVerifyTLS
	fieldCount
)

// fieldInfo describes one onboarding input: its label, a help line shown above
// the input, and the textinput placeholder.
type fieldInfo struct {
	label       string
	desc        string
	placeholder string
	mask        bool
}

var fieldInfos = [fieldCount]fieldInfo{
	{
		label:       "Cluster Endpoints (required)",
		desc:        "Comma-separated PVE API URLs (including /api2/json), tried in order so a down node falls through to the next.",
		placeholder: "https://pve1.example.com:8006/api2/json, https://pve2.example.com:8006/api2/json",
	},
	{
		label:       "API Token ID (required)",
		desc:        "Full token identifier in the form user@realm!tokenname.",
		placeholder: "inventory@pve!readonly",
	},
	{
		label:       "API Token Secret (required)",
		desc:        "The secret half of the API token.",
		placeholder: "••••••••••••",
		mask:        true,
	},
	{
		label:       "Default SSH User (required)",
		desc:        "SSH user for every VM that has no per-VM override.",
		placeholder: "root",
	},
	{
		label:       "SSH Command Template (optional)",
		desc:        "Template with {{user}} and {{ip}} placeholders. Leave empty for the default.",
		placeholder: "ssh {{user}}@{{ip}}",
	},
	{
		label: "Verify TLS Certificates (optional)",
		desc:  "Uncheck for clusters using self-signed certificates. Space toggles.",
	},
}

// tokenPerms is the explanation shown below the API Token Secret field,
// listing the permissions the token must have for the TUI to work.
const tokenPerms = `Required token permissions:
  • Sys.Audit on /                    — list cluster resources
  • VM.Audit on /vms                  — read VM status and config
  • VM.GuestAgent.Audit on /vms       — query the guest agent for IPs
    (VM.Monitor on PVE 8.x, which lacks VM.GuestAgent.Audit)

PVE 9.x: the built-in PVEAuditor role grants all three.
PVE 8.x: PVEAuditor lacks VM.Monitor, so add a custom role
         (e.g. GuestAgentAudit with VM.Monitor) on /vms.

With privilege separation on, the token's effective permissions are the
intersection of the user's and the token's ACLs, so assign every role to
BOTH the user and the token.`

// form holds the onboarding form state: the textinput slice, the focused
// field index, a boolean toggle for the TLS checkbox, an error string shown
// at the bottom, and a viewport that scrolls the form when the terminal is
// too short to show every field at once. fieldLines[i] is the 0-based line
// index of field i's input row within the built content — used to keep the
// focused field visible after Tab/Shift+Tab.
type form struct {
	inputs     []textinput.Model
	focus      int
	verifyTLS  bool
	err        string
	viewport   viewport.Model
	fieldLines [fieldCount]int
}

// newForm builds the onboarding form with prefilled defaults from
// config.Default(). Each textinput is given a filled background style so the
// editable area reads as a box rather than prose; the focused field uses a
// brighter background than blurred fields.
func newForm() form {
	def := config.Default()
	inputs := make([]textinput.Model, fieldCount)

	for i := 0; i < fieldCount; i++ {
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetWidth(60)
		ti.CharLimit = 200

		switch i {
		case fieldEndpoints:
			ti.Placeholder = fieldInfos[i].placeholder
			ti.SetValue(strings.Join(def.Endpoints, ", "))
		case fieldTokenID:
			ti.Placeholder = fieldInfos[i].placeholder
		case fieldTokenSecret:
			ti.Placeholder = fieldInfos[i].placeholder
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		case fieldSSHUser:
			ti.Placeholder = fieldInfos[i].placeholder
			ti.SetValue(def.DefaultSSHUser)
		case fieldSSHTemplate:
			ti.Placeholder = fieldInfos[i].placeholder
			ti.SetValue(def.SSHCommandTemplate)
		}

		inputs[i] = ti
	}

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))
	vp.SoftWrap = true
	vp.MouseWheelEnabled = true

	f := form{inputs: inputs, verifyTLS: true, viewport: vp}
	f.inputs[f.focus].Focus()
	return f
}

// onboardingUpdate handles input for the onboarding form and the connection
// test. During validation only the spinner tick and the result message are
// processed — all other input is queued until the test finishes.
func (m model) onboardingUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.state == stateOnboardingValidating {
		if msg, ok := msg.(vmsLoadedMsg); ok {
			if msg.err != nil {
				m.state = stateOnboarding
				m.form.err = "Connection failed: " + msg.err.Error()
				return m, m.form.inputs[m.form.focus].Focus()
			}
			if err := config.Save(&m.cfg); err != nil {
				m.state = stateOnboarding
				m.form.err = "Failed to save config: " + err.Error()
				return m, m.form.inputs[m.form.focus].Focus()
			}
			m.state = stateVMListReady
			m.vms = msg.vms
			m.selected = 0
			m.hovered = -1
			m.scroll = 0
			_ = cache.Save(msg.vms)
			return m, nil
		}
		return m, nil
	}

	// stateOnboarding — handle form navigation and input.
	// Sync the viewport first so scroll operations (EnsureVisible, PageUp,
	// PageDown) see a correct maxYOffset.
	m.syncOnboardingViewport()

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			return m.submitOnboarding()
		case "tab":
			if m.form.focus != fieldVerifyTLS {
				m.form.inputs[m.form.focus].Blur()
			}
			m.form.focus = (m.form.focus + 1) % fieldCount
			if m.form.focus != fieldVerifyTLS {
				m.ensureFieldVisible()
				return m, m.form.inputs[m.form.focus].Focus()
			}
			m.ensureFieldVisible()
			return m, nil
		case "shift+tab":
			if m.form.focus != fieldVerifyTLS {
				m.form.inputs[m.form.focus].Blur()
			}
			m.form.focus = (m.form.focus - 1 + fieldCount) % fieldCount
			if m.form.focus != fieldVerifyTLS {
				m.ensureFieldVisible()
				return m, m.form.inputs[m.form.focus].Focus()
			}
			m.ensureFieldVisible()
			return m, nil
		case "esc":
			return m, tea.Quit
		case "space":
			if m.form.focus == fieldVerifyTLS {
				m.form.verifyTLS = !m.form.verifyTLS
				return m, nil
			}
		case "pgup":
			m.form.viewport.PageUp()
			return m, nil
		case "pgdown":
			m.form.viewport.PageDown()
			return m, nil
		}
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		m.form.viewport, cmd = m.form.viewport.Update(msg)
		return m, cmd
	}

	// Route any other message to the focused textinput. The checkbox has no
	// textinput and consumes only the space toggle handled above.
	if m.form.focus != fieldVerifyTLS {
		var cmd tea.Cmd
		m.form.inputs[m.form.focus], cmd = m.form.inputs[m.form.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

// submitOnboarding reads the form values into a Config, validates locally,
// and — on success — kicks off a background connection test. The state
// transitions to stateOnboardingValidating so the form locks until the test
// returns.
func (m model) submitOnboarding() (tea.Model, tea.Cmd) {
	m.cfg = config.Config{
		Endpoints:          splitCSV(m.form.inputs[fieldEndpoints].Value()),
		APITokenID:         m.form.inputs[fieldTokenID].Value(),
		APITokenSecret:     m.form.inputs[fieldTokenSecret].Value(),
		Insecure:           !m.form.verifyTLS,
		DefaultSSHUser:     m.form.inputs[fieldSSHUser].Value(),
		SSHCommandTemplate: m.form.inputs[fieldSSHTemplate].Value(),
	}

	if err := m.cfg.Validate(); err != nil {
		m.form.err = "Invalid config: " + err.Error()
		return m, nil
	}

	m.state = stateOnboardingValidating
	m.form.err = ""
	return m, tea.Batch(m.spinner.Tick, loadVMsCmd(m.cfg))
}

// onboardingView renders two stacked bordered boxes: a scrollable form pane
// on top (viewport-driven so it scrolls when the terminal is too short) and a
// fixed keybindings pane below that stays visible at all times. The viewport's
// yOffset is preserved across renders, so scroll position set by
// Tab/Shift+Tab/PgUp/PgDn/mouse wheel persists.
func (m model) onboardingView() string {
	w, h := m.width, m.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}

	if m.state == stateOnboardingValidating {
		mainBox := styleFrame.Width(w).Height(h - hintBoxHeight).Render(
			m.spinner.View() + "  Connecting to cluster and loading VMs...")
		return lipgloss.JoinVertical(lipgloss.Left, mainBox, m.hintBox(w))
	}

	content, _ := m.buildOnboardingContent()

	// Work on a copy of the viewport: View() is a value receiver, so the
	// real model's viewport (whose yOffset was set by Update) is not
	// mutated. SetContent on the copy preserves the copied yOffset.
	vp := m.form.viewport
	m.sizeOnboardingViewport(&vp)
	vp.SetContent(content)

	mainBox := styleFrame.Width(w).Height(h - hintBoxHeight).Render(vp.View())
	return lipgloss.JoinVertical(lipgloss.Left, mainBox, m.hintBox(w))
}

// hintBoxHeight is the vertical space reserved for the keybindings pane:
// 1 blank separator + 1 blank line + 1 content line = 3.
const hintBoxHeight = 3

// hintBox renders the always-visible keybindings line.
func (m model) hintBox(width int) string {
	hints := "Tab: next field  Shift+Tab: prev  Space: toggle  Enter: submit  PgUp/PgDn: scroll  Esc: quit"
	return "\n" + styleHint.Padding(0, 2).Width(width).Render(hints)
}

// syncOnboardingViewport rebuilds the form content, records each field's
// input-row line index, and pushes both the content and the current terminal
// dimensions into the viewport. Called from Update so that EnsureVisible,
// PageUp, and PageDown operate against a correct maxYOffset.
func (m *model) syncOnboardingViewport() {
	content, fieldLines := m.buildOnboardingContent()
	m.form.fieldLines = fieldLines
	m.sizeOnboardingViewport(&m.form.viewport)
	m.form.viewport.SetContent(content)
}

// sizeOnboardingViewport sets the viewport width/height to the interior of
// the main frame. The frame adds a 1-cell border on every side and 2 cells of
// horizontal padding, so the interior is width-6 by (mainBoxHeight-2).
// mainBoxHeight is the terminal height minus the reserved hintBoxHeight.
func (m model) sizeOnboardingViewport(vp *viewport.Model) {
	fw, fh := m.width, m.height
	if fw == 0 {
		fw = 80
	}
	if fh == 0 {
		fh = 24
	}
	fh -= hintBoxHeight
	fw -= 6
	fh -= 2
	if fw < 1 {
		fw = 1
	}
	if fh < 1 {
		fh = 1
	}
	vp.SetWidth(fw)
	vp.SetHeight(fh)
}

// ensureFieldVisible scrolls the viewport so the focused field's input row is
// visible.
func (m *model) ensureFieldVisible() {
	if m.form.focus < 0 || m.form.focus >= fieldCount {
		return
	}
	m.form.viewport.EnsureVisible(m.form.fieldLines[m.form.focus], 0, 0)
}

// buildOnboardingContent renders the full onboarding form as a single string
// and returns it together with fieldLines, where fieldLines[i] is the 0-based
// line index of field i's input row within the returned content. The line
// structure is static (input values never add or remove lines), so the
// indices stay valid across renders.
func (m model) buildOnboardingContent() (string, [fieldCount]int) {
	var b strings.Builder
	var fieldLines [fieldCount]int
	line := 0

	b.WriteString(styleTitle.Render("ssh-pve — Onboarding"))
	b.WriteString("\n\n")
	line += 2

	// Fit the input width to the terminal. The available width is the frame
	// interior (m.width-6) minus the 2-char indent (reserved for the focus
	// indicator) minus the input border (4 = 2 cells per side).
	inputWidth := m.width - 6 - 2 - 4
	if inputWidth > 60 {
		inputWidth = 60
	}
	if inputWidth < 20 {
		inputWidth = 20
	}
	for i := range m.form.inputs {
		m.form.inputs[i].SetWidth(inputWidth)
	}

	for i := 0; i < fieldCount; i++ {
		info := fieldInfos[i]

		// Label
		b.WriteString(styleLabel.Render(info.label))
		b.WriteString("\n")
		line++

		// Description
		b.WriteString(styleDesc.Render(info.desc))
		b.WriteString("\n")
		line++

		// Extra permission notice after the token secret field.
		if i == fieldTokenSecret {
			b.WriteString("\n")
			line++
			for _, l := range strings.Split(tokenPerms, "\n") {
				b.WriteString(styleDesc.Render(l))
				b.WriteString("\n")
				line++
			}
		}

		// Field value. The box is indented 2 chars so the focus indicator
		// (rendered on the box's middle line) never shifts the top border.
		b.WriteString("\n")
		line++
		fieldLines[i] = line

		border := styleInputBorder
		if i == m.form.focus {
			border = styleInputBorderFocus
		}
		var box string
		if i == fieldVerifyTLS {
			box = "[ ] Verify TLS certificates"
			if m.form.verifyTLS {
				box = "[x] Verify TLS certificates"
			}
		} else {
			box = m.form.inputs[i].View()
		}
		boxLines := strings.Split(border.Render(box), "\n")
		for j, bl := range boxLines {
			b.WriteString("  ")
			// Overlay the focus indicator on the middle line of the box.
			if i == m.form.focus && j == len(boxLines)/2 {
				b.WriteString(styleSelected.Render("▸"))
			} else {
				b.WriteString(" ")
			}
			b.WriteString(bl)
			b.WriteString("\n")
			line++
		}
		b.WriteString("\n")
		line++
	}

	// Error
	if m.form.err != "" {
		b.WriteString(styleError.Render(m.form.err))
		b.WriteString("\n\n")
	}

	return b.String(), fieldLines
}
