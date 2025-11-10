package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// PassphraseModal is a simple modal for prompting SSH key passphrase
// Similar to ErrorModal but with password input field
type PassphraseModal struct {
	keyPath   string
	input     textinput.Model
	err       string
	width     int
	height    int
	cancelled bool
}

func NewPassphraseModal(keyPath string) PassphraseModal {
	input := NewPassphraseInput("Enter passphrase")
	input.Focus()

	return PassphraseModal{
		keyPath: keyPath,
		input:   input,
	}
}

func (m PassphraseModal) Init() tea.Cmd {
	return textinput.Blink
}

func (m PassphraseModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit

		case "enter":
			if err := ValidatePassphraseNotEmpty(m.input.Value()); err != nil {
				m.err = GetEmptyPassphraseError()
				return m, nil
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m PassphraseModal) View() string {
	return RenderPassphraseModal(
		"SSH Key Passphrase Required",
		m.keyPath,
		m.input,
		m.err,
		m.width,
		m.height,
	)
}

// GetPassphrase returns the entered passphrase (empty if cancelled)
func (m PassphraseModal) GetPassphrase() string {
	if m.cancelled {
		return ""
	}
	return m.input.Value()
}

// IsCancelled returns true if user pressed Esc
func (m PassphraseModal) IsCancelled() bool {
	return m.cancelled
}
