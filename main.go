package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"otui/config"
	"otui/storage"
	"otui/ui"
)

const (
	Version = "v0.04.00"
	License = "Apache-2.0"
)

// buildCredentialErrorMessage builds a helpful error message for credential failures
func buildCredentialErrorMessage(err error) string {
	var msg strings.Builder

	msg.WriteString("Failed to load credentials.\n\n")

	if strings.Contains(err.Error(), "failed to decrypt") {
		msg.WriteString("The credentials file may be corrupted or encrypted\n")
		msg.WriteString("with a different SSH key.\n\n")
		msg.WriteString("To fix:\n")
		msg.WriteString("1. Delete: ~/.local/share/otui/credentials.enc\n")
		msg.WriteString("2. Restart OTUI and re-enter your API keys")
	} else {
		msg.WriteString(fmt.Sprintf("Error: %v\n\n", err))
		msg.WriteString("Please check your configuration.")
	}

	return msg.String()
}

// extractSSHKeyPath attempts to extract SSH key path from config
// If config can't be loaded, returns default path
func extractSSHKeyPath() string {
	settingsPath := config.GetSettingsFilePath()
	if !config.FileExists(settingsPath) {
		return "~/.ssh/otui_ed25519"
	}

	systemCfg, err := config.LoadSystemConfig()
	if err != nil {
		return "~/.ssh/otui_ed25519"
	}

	dataDir := config.ExpandPath(systemCfg.DataDirectory)
	userCfg, err := config.LoadUserConfigFromPath(filepath.Join(dataDir, "config.toml"))
	if err != nil || userCfg == nil {
		return "~/.ssh/otui_ed25519"
	}

	if userCfg.Security.SSHKeyPath != "" {
		return userCfg.Security.SSHKeyPath
	}

	return "~/.ssh/otui_ed25519"
}

func main() {
	// Initialize early debug logging (writes to cache dir)
	config.InitEarlyDebugLog()

	// Validate environment variables first
	if config.HasAnyEnvVar() && !config.HasAllEnvVars() {
		missingVar := config.GetMissingEnvVar()
		errorMsg := fmt.Sprintf("Missing environment variable: %s\n\n"+
			"When using environment variables, all 3 must be set:\n"+
			"  • OTUI_OLLAMA_HOST\n"+
			"  • OTUI_OLLAMA_MODEL\n"+
			"  • OTUI_DATA_DIR\n\n"+
			"Set the missing variable(s) before launching otui.",
			missingVar)

		errorModal := ui.NewErrorModal("Configuration Error", errorMsg)
		p := tea.NewProgram(
			errorModal,
			tea.WithAltScreen(),
		)

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(0)
	}

	settingsPath := config.GetSettingsFilePath()
	isFirstRun := !config.FileExists(settingsPath)

	// Skip welcome wizard if all env vars are set
	if config.HasAllEnvVars() {
		isFirstRun = false
	}

	// NEW: Also check if user config exists in data directory
	// This handles the case where system config exists (data dir was set in Settings)
	// but user config doesn't exist yet (new data dir needs to be created via wizard)
	if !isFirstRun {
		systemCfg, err := config.LoadSystemConfig()
		if err == nil {
			dataDir := config.ExpandPath(systemCfg.DataDirectory)
			userConfigPath := filepath.Join(dataDir, "config.toml")
			if !config.FileExists(userConfigPath) {
				// System config exists but user config doesn't - run welcome wizard
				isFirstRun = true
			}
		}
	}

	if isFirstRun {
		welcomeModel := ui.NewWelcomeModel()
		p := tea.NewProgram(
			welcomeModel,
			tea.WithAltScreen(),
		)

		finalModel, err := p.Run()
		if err != nil {
			fmt.Printf("Error running welcome wizard: %v\n", err)
			os.Exit(1)
		}

		if wm, ok := finalModel.(ui.WelcomeModel); ok && !wm.IsComplete() {
			os.Exit(0)
		}
	}

	// Try to load config
	cfg, err := config.Load()

	// Track whether or not the user input the right passphrase
	pass := false

	// If SSH key passphrase is required, prompt for it in a loop
	for err != nil && strings.Contains(err.Error(), "passphrase required") {
		keyPath := extractSSHKeyPath()

		// Show passphrase modal
		modal := ui.NewPassphraseModal(keyPath)
		p := tea.NewProgram(modal, tea.WithAltScreen())

		finalModel, err := p.Run()
		if err != nil {
			fmt.Printf("Error running passphrase prompt: %v\n", err)
			os.Exit(1)
		}

		modal = finalModel.(ui.PassphraseModal)

		if modal.IsCancelled() {
			fmt.Println("Passphrase prompt cancelled. Exiting.")
			os.Exit(0)
		}

		passphrase := modal.GetPassphrase()

		// Try to load config with passphrase using helper
		perr := ui.LoadCredentialsWithPassphrase(cfg, passphrase)

		// If no error (success) or different error, break out of loop
		if perr == nil {
			pass = true
			break
		}
	}

	// Handle other config loading errors
	if err != nil && pass == false {
		var errorMsg string

		if strings.Contains(err.Error(), "failed to load credentials") {
			errorMsg = buildCredentialErrorMessage(err)
		} else {
			errorMsg = fmt.Sprintf("Failed to load config:\n\n%v", err)
		}

		errorModal := ui.NewErrorModal("Configuration Error", errorMsg)
		p := tea.NewProgram(errorModal, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	// Initialize debug logging after config is loaded
	config.InitDebugLog(cfg.DataDir())

	// Clean up old tmp dir in cache directory (crash recovery)
	if err := config.CleanupTempDir(); err != nil && config.DebugLog != nil {
		config.DebugLog.Printf("Warning: failed to cleanup old temp directory: %v", err)
	}

	// Create secure temp directory in cache (never synced to cloud)
	if err := config.CreateTempDir(); err != nil {
		fmt.Printf("Failed to create secure temp directory: %v\n", err)
		os.Exit(1)
	}

	// Ensure cleanup on exit
	defer func() {
		if err := config.CleanupTempDir(); err != nil && config.DebugLog != nil {
			config.DebugLog.Printf("Warning: failed to cleanup temp directory on exit: %v", err)
		}
	}()

	sessionStorage, err := storage.NewSessionStorage(cfg.DataDir())
	if err != nil {
		fmt.Printf("Failed to initialize session storage: %v\n", err)
		os.Exit(1)
	}

	// Check if another OTUI instance is already running (single-instance enforcement)
	isLocked, runningPID, err := sessionStorage.CheckOTUIInstanceLock()
	if err != nil {
		fmt.Printf("Failed to check instance lock: %v\n", err)
		os.Exit(1)
	}
	if isLocked {
		// Another instance is running - show error modal and exit
		errorMsg := fmt.Sprintf(
			"Another OTUI instance is already running (PID %d).\n\n"+
				"Only one instance of OTUI can run per system.\n\n"+
				"To run multiple instances:\n"+
				"• Use system containers (Incus/Podman)\n"+
				"• Each container provides isolated environment\n\n"+
				"Close the other instance or run OTUI in a container.",
			runningPID)

		errorModal := ui.NewErrorModal("⚠️  OTUI Already Running  ⚠️", errorMsg)
		p := tea.NewProgram(
			errorModal,
			tea.WithAltScreen(),
		)

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(0)
	}

	// Lock this instance
	if err := sessionStorage.LockOTUIInstance(); err != nil {
		fmt.Printf("Failed to lock OTUI instance: %v\n", err)
		os.Exit(1)
	}

	// Load last session with lock check
	var lastSession *storage.Session
	if lastSessionID, err := sessionStorage.LoadCurrentSessionID(); err == nil {
		// Check if session is locked by another instance
		isLocked, lockErr := sessionStorage.CheckSessionLock(lastSessionID)
		if lockErr == nil && !isLocked {
			lastSession, _ = sessionStorage.Load(lastSessionID)
			// Note: Lock will be acquired when setCurrentSession is called in UI
		}
		// If locked: lastSession remains nil → NewModel will create new session
	}

	// Create AppView before defer so we can unlock the CURRENT data directory on exit
	// (which may differ from initial data directory if user switched during session)
	appView := ui.NewAppView(cfg, sessionStorage, lastSession, Version, License)

	// Ensure cleanup on exit - uses appView method to unlock CURRENT data directory
	defer func() {
		if err := appView.UnlockCurrentDataDir(); err != nil && config.DebugLog != nil {
			config.DebugLog.Printf("Warning: failed to unlock OTUI instance: %v", err)
		}
	}()

	p := tea.NewProgram(
		appView,
		tea.WithAltScreen(),
	)

	finalModel, err := p.Run()
	if err != nil {
		fmt.Printf("Error running otui: %v\n", err)
		os.Exit(1)
	}

	// Check if we should restart OTUI (for creating new data directories)
	if appView, ok := finalModel.(ui.AppView); ok && appView.RestartAfterQuit {
		if config.DebugLog != nil {
			config.DebugLog.Printf("[main] RestartAfterQuit flag detected - restarting OTUI")
		}

		binary, err := os.Executable()
		if err != nil {
			fmt.Printf("Failed to get executable path: %v\n", err)
			os.Exit(1)
		}

		// Restart process (replaces current process)
		if err := syscall.Exec(binary, []string{binary}, os.Environ()); err != nil {
			fmt.Printf("Failed to restart OTUI: %v\n", err)
			os.Exit(1)
		}
	}
}
