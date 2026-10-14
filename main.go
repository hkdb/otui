package main

import (
	"errors"
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
	Version = "v0.08.02"
	License = "Apache-2.0"
)

// buildCredentialErrorMessage builds a helpful error message for credential failures
func buildCredentialErrorMessage(err error) string {
	var msg strings.Builder

	msg.WriteString("Failed to load credentials.\n\n")

	dataDir := credentialsDataDir()

	if errors.Is(err, config.ErrUnsupportedKeyType) {
		msg.WriteString("This SSH key type can't be used to encrypt credentials.\n")
		msg.WriteString("OTUI needs an ed25519 key (RSA keys also work).\n\n")
		msg.WriteString("To fix:\n")
		msg.WriteString("1. Create a key: ssh-keygen -t ed25519 -f ~/.ssh/otui_ed25519\n")
		msg.WriteString(fmt.Sprintf("2. Set ssh_key_path in %s\n", filepath.Join(dataDir, "config.toml")))
		msg.WriteString(fmt.Sprintf("3. Delete %s\n", filepath.Join(dataDir, "credentials.enc")))
		msg.WriteString("4. Restart OTUI and re-enter your API keys")
	} else if strings.Contains(err.Error(), "failed to decrypt") {
		msg.WriteString("The credentials file may be corrupted or encrypted\n")
		msg.WriteString("with a different SSH key.\n\n")
		msg.WriteString("To fix:\n")
		msg.WriteString(fmt.Sprintf("1. Delete: %s\n", filepath.Join(dataDir, "credentials.enc")))
		msg.WriteString("2. Restart OTUI and re-enter your API keys")
	} else {
		msg.WriteString(fmt.Sprintf("Error: %v\n\n", err))
		msg.WriteString("Please check your configuration.")
	}

	return msg.String()
}

// credentialsDataDir returns the data directory for error messages: the
// OTUI_DATA_DIR/settings.toml one, or the default
func credentialsDataDir() string {
	if env, err := config.ReadEnvConfig(); err == nil && env != nil {
		return config.ExpandPath(env.DataDir)
	}
	if config.SystemConfigExists() {
		if systemCfg, err := config.LoadSystemConfig(); err == nil {
			return config.ExpandPath(systemCfg.DataDirectory)
		}
	}
	return config.GetDefaultDataDir()
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

// showConfigError shows a configuration error in a modal and returns when
// the user dismisses it
func showConfigError(msg string) {
	p := tea.NewProgram(ui.NewErrorModal("Configuration Error", msg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
}

func main() {
	// Initialize early debug logging (writes to cache dir)
	config.InitEarlyDebugLog()

	// Validate environment variables first (OTUI_* configure OTUI without
	// the setup wizard, e.g. in containers)
	envCfg, envErr := config.ReadEnvConfig()
	if envErr != nil {
		showConfigError(fmt.Sprintf("Invalid environment configuration:\n%v\n\n%s", envErr, config.EnvUsage))
		os.Exit(1)
	}

	settingsPath := config.GetSettingsFilePath()
	isFirstRun := !config.FileExists(settingsPath)

	// Skip welcome wizard if configured through environment variables
	if envCfg != nil {
		isFirstRun = false
	}

	// NEW: Also check if user config exists in data directory
	// This handles the case where system config exists (data dir was set in Settings)
	// but user config doesn't exist yet (new data dir needs to be created via wizard)
	if !isFirstRun && envCfg == nil {
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
		// Load default keybindings for wizard
		kb := config.DefaultKeybindings()
		welcomeModel := ui.NewWelcomeModel(kb)
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

	// If SSH key passphrase is required, prompt for it until it's correct
	promptErr := ""
	needPassphrase := err != nil && strings.Contains(err.Error(), "passphrase required")
	for needPassphrase {
		keyPath := extractSSHKeyPath()

		// Show passphrase modal (with the previous attempt's error, if any)
		p := tea.NewProgram(ui.NewPassphraseModalWithError(keyPath, promptErr), tea.WithAltScreen())

		finalModel, runErr := p.Run()
		if runErr != nil {
			fmt.Printf("Error running passphrase prompt: %v\n", runErr)
			os.Exit(1)
		}

		modal, ok := finalModel.(ui.PassphraseModal)
		if !ok || modal.IsCancelled() {
			fmt.Println("Passphrase prompt cancelled. Exiting.")
			os.Exit(0)
		}

		// Try to load credentials with the passphrase
		perr := ui.LoadCredentialsWithPassphrase(cfg, modal.GetPassphrase())
		switch {
		case perr == nil:
			err = nil
			needPassphrase = false
		case config.IsIncorrectPassphrase(perr):
			// Ask again
			promptErr = ui.GetIncorrectPassphraseError()
		default:
			// Not a passphrase problem (e.g. credentials encrypted with a
			// different key) - show it instead of prompting forever
			err = fmt.Errorf("failed to load credentials: %w", perr)
			needPassphrase = false
		}
	}

	// Handle other config loading errors
	if err != nil {
		var errorMsg string

		if strings.Contains(err.Error(), "failed to load credentials") {
			errorMsg = buildCredentialErrorMessage(err)
		} else {
			errorMsg = fmt.Sprintf("Failed to load config:\n\n%v", err)
		}

		showConfigError(errorMsg)
		os.Exit(1)
	}

	// A cloud provider configured through the environment needs an API key,
	// either OTUI_API_KEY or one stored in the data directory
	if cfg.Env != nil && cfg.Env.RequiresAPIKey() && cfg.APIKey(cfg.Env.Provider) == "" {
		showConfigError(fmt.Sprintf("No API key for %s.\n\nSet OTUI_API_KEY, or store a key for %s\nin the data directory (%s).",
			cfg.Env.Provider, cfg.Env.Provider, cfg.DataDir()))
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
		// Another instance is running - show modal with option to force delete lock file
		lockModal := ui.NewInstanceLockedModal(runningPID)
		p := tea.NewProgram(
			lockModal,
			tea.WithAltScreen(),
		)

		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		modal := finalModel.(ui.InstanceLockedModal)
		if modal.ForceDelete() {
			// User chose to force delete the lock file
			if err := sessionStorage.UnlockOTUIInstance(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to delete lock file: %v\n", err)
				os.Exit(1)
			}
			// Continue with normal startup
		} else {
			// User chose to exit
			os.Exit(0)
		}
	}

	// Lock this instance
	if err := sessionStorage.LockOTUIInstance(); err != nil {
		fmt.Printf("Failed to lock OTUI instance: %v\n", err)
		os.Exit(1)
	}

	// Load last session with lock check
	var lastSession *storage.Session
	var lockedSessionID string
	if lastSessionID, err := sessionStorage.LoadCurrentSessionID(); err == nil {
		// Check if session is locked by another instance
		isLocked, lockErr := sessionStorage.CheckSessionLock(lastSessionID)
		if lockErr == nil && !isLocked {
			lastSession, _ = sessionStorage.Load(lastSessionID)
			if lastSession != nil {
				_ = sessionStorage.LockSession(lastSession.ID)
			}
		} else if lockErr == nil && isLocked {
			// lastSession remains nil → NewAppView creates a new session and
			// offers to force delete the stale lock
			lockedSessionID = lastSessionID
		}
	}

	// Create AppView before defer so we can unlock the CURRENT data directory on exit
	// (which may differ from initial data directory if user switched during session)
	appView := ui.NewAppView(cfg, sessionStorage, lastSession, lockedSessionID, Version, License)

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
