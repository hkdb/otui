package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"otui/config"
)

// ===== INPUT CREATION =====

// NewPassphraseInput creates a configured textinput for SSH passphrase entry
// Reusable across all passphrase entry contexts (launch, welcome wizard, data dir switch)
func NewPassphraseInput(placeholder string) textinput.Model {
	input := textinput.New()
	input.Placeholder = placeholder
	input.Width = 50
	input.CharLimit = 200
	input.EchoMode = textinput.EchoPassword
	input.EchoCharacter = '•'
	return input
}

// ===== RENDERING =====

// RenderPassphraseModal renders a modal prompting for SSH key passphrase
// Reusable by modal_passphrase.go, welcome.go, and appview.go
func RenderPassphraseModal(
	title string,
	keyPath string,
	passphraseInput textinput.Model,
	errorMsg string,
	width int,
	height int,
) string {
	// Guard clause: prevent rendering in tiny terminals or before WindowSizeMsg
	if width < 20 || height < 10 {
		return "Terminal too small"
	}

	modalWidth := 70
	if width < modalWidth+10 {
		modalWidth = width - 10
		if modalWidth < 10 {
			modalWidth = 10
		}
	}

	var messageLines []string
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Message text
	msg1 := "The SSH key is encrypted with a passphrase."
	msg2 := fmt.Sprintf("Key: %s", keyPath)
	msg3 := "Please enter the passphrase:"

	messageLines = append(messageLines, centerTextLine(msg1, modalWidth))
	messageLines = append(messageLines, centerTextLine(msg2, modalWidth))
	messageLines = append(messageLines, centerTextLine(msg3, modalWidth))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Input field
	messageLines = append(messageLines, centerTextLine(passphraseInput.View(), modalWidth))
	messageLines = append(messageLines, strings.Repeat(" ", modalWidth))

	// Error message if present
	if errorMsg != "" {
		errLine := "⚠ " + errorMsg
		styledErr := lipgloss.NewStyle().
			Foreground(dangerColor).
			Bold(true).
			Render(errLine)
		messageLines = append(messageLines, centerTextLine(styledErr, modalWidth))
		messageLines = append(messageLines, strings.Repeat(" ", modalWidth))
	}

	footer := "Enter Continue  |  Esc Cancel"

	return RenderThreeSectionModal(
		title,
		messageLines,
		footer,
		ModalTypeInfo,
		modalWidth,
		width,
		height,
	)
}

// centerTextLine centers a line of text within a given width
// Helper for passphrase modal rendering
func centerTextLine(text string, width int) string {
	textWidth := lipgloss.Width(text)
	if textWidth >= width {
		return text
	}

	leftPad := (width - textWidth) / 2
	rightPad := width - textWidth - leftPad
	return strings.Repeat(" ", leftPad) + text + strings.Repeat(" ", rightPad)
}

// ===== VALIDATION =====

// ValidatePassphraseNotEmpty validates that a passphrase is not empty
// Returns error if empty, nil if valid
func ValidatePassphraseNotEmpty(passphrase string) error {
	if passphrase == "" {
		return fmt.Errorf("passphrase cannot be empty")
	}
	return nil
}

// ===== ERROR MESSAGES =====

// GetEmptyPassphraseError returns the standard error message for empty passphrase
// Single source of truth for this error message across all contexts
func GetEmptyPassphraseError() string {
	return "Passphrase cannot be empty"
}

// GetIncorrectPassphraseError returns the standard error message for incorrect passphrase
// Single source of truth for this error message across all contexts
func GetIncorrectPassphraseError() string {
	return "Incorrect passphrase. Please try again."
}

// ===== CREDENTIAL LOADING =====

// LoadCredentialsWithPassphrase is a UI orchestration helper that loads credentials
// with the provided passphrase. This is a convenience wrapper around config methods.
//
// Returns nil on success, error on failure (wrong passphrase or other errors)
func LoadCredentialsWithPassphrase(cfg *config.Config, passphrase string) error {
	// Validate we have a config with credential store
	if cfg == nil || cfg.CredentialStore == nil {
		return fmt.Errorf("invalid config - cannot set passphrase")
	}

	// Set the passphrase on the credential store
	cfg.CredentialStore.SetPassphrase(passphrase)

	// Try to load credentials (will fail if passphrase is wrong)
	dataDir := cfg.DataDir()
	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[modal_passphrase_helpers] LoadCredentialsWithPassphrase: Attempting to load credentials")
	}

	if err := cfg.CredentialStore.Load(dataDir); err != nil {
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[modal_passphrase_helpers] LoadCredentialsWithPassphrase: Failed to load credentials: %v", err)
		}
		return err
	}

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[modal_passphrase_helpers] LoadCredentialsWithPassphrase: Successfully loaded credentials")
	}

	return nil
}
