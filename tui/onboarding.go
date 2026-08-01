package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"

	"github.com/pneugebala/ssh-pve/cache"
	"github.com/pneugebala/ssh-pve/config"
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
const tokenPerms = `Required token permissions (PVE 8.x+):
  • Sys.Audit on /                    — list cluster resources
  • VM.Audit on /vms                  — read VM status and config
  • VM.GuestAgent.Audit on /vms       — query the guest agent for IPs

The built-in PVEAuditor role grants the first two but NOT the third.
Create a custom role (e.g. GuestAgentReader with VM.GuestAgent.Audit)
and assign it on /vms alongside PVEAuditor on /.`

// form holds the onboarding form state: the textinput slice, the focused
// field index, a boolean toggle for the TLS checkbox, and an error string
// shown at the bottom.
type form struct {
	inputs    []textinput.Model
	focus     int
	verifyTLS bool
	err       string
}

// newForm builds the onboarding form with prefilled defaults from
// config.Default().
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

	f := form{inputs: inputs, verifyTLS: true}
	f.inputs[f.focus].Focus()
	return f
}

// onboardingUpdate handles input for the onboarding form and the connection
// test. During validation only the spinner tick and the result message are
// processed — all other input is queued until the test finishes.
func (m model) onboardingUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.state == stateOnboardingValidating {
		switch msg := msg.(type) {
		case vmsLoadedMsg:
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
				return m, m.form.inputs[m.form.focus].Focus()
			}
			return m, nil
		case "shift+tab":
			if m.form.focus != fieldVerifyTLS {
				m.form.inputs[m.form.focus].Blur()
			}
			m.form.focus = (m.form.focus - 1 + fieldCount) % fieldCount
			if m.form.focus != fieldVerifyTLS {
				return m, m.form.inputs[m.form.focus].Focus()
			}
			return m, nil
		case "esc":
			return m, tea.Quit
		case "space":
			if m.form.focus == fieldVerifyTLS {
				m.form.verifyTLS = !m.form.verifyTLS
				return m, nil
			}
		}
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

// onboardingView renders the full onboarding form with field labels,
// descriptions, inputs, the token permission notice, and a hint bar.
func (m model) onboardingView() string {
	var b strings.Builder

	if m.state == stateOnboardingValidating {
		b.WriteString(m.spinner.View())
		b.WriteString("  Connecting to cluster and loading VMs...")
	} else {
		b.WriteString(styleTitle.Render("ssh-pve — Onboarding"))
		b.WriteString("\n\n")

		for i := 0; i < fieldCount; i++ {
			info := fieldInfos[i]

			// Label
			b.WriteString(styleLabel.Render(info.label))
			b.WriteString("\n")

			// Description
			b.WriteString(styleDesc.Render(info.desc))
			b.WriteString("\n")

			// Extra permission notice after the token secret field.
			if i == fieldTokenSecret {
				b.WriteString("\n")
				for _, line := range strings.Split(tokenPerms, "\n") {
					b.WriteString(styleDesc.Render(line))
					b.WriteString("\n")
				}
			}

			// Field value (with focus indicator)
			b.WriteString("\n")
			if i == m.form.focus {
				b.WriteString(styleSelected.Render("▸ "))
			} else {
				b.WriteString("  ")
			}
			if i == fieldVerifyTLS {
				if m.form.verifyTLS {
					b.WriteString("[x] Verify TLS certificates")
				} else {
					b.WriteString("[ ] Verify TLS certificates")
				}
			} else {
				b.WriteString(m.form.inputs[i].View())
			}
			b.WriteString("\n\n")
		}

		// Error
		if m.form.err != "" {
			b.WriteString(styleError.Render(m.form.err))
			b.WriteString("\n\n")
		}

		// Hints
		b.WriteString(styleHint.Render("Tab: next field  Shift+Tab: prev  Space: toggle  Enter: submit  Esc: quit"))
	}

	return styleFrame.Width(m.width).Height(m.height).Render(b.String())
}
