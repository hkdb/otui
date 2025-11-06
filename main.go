package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"otui/config"
	"otui/storage"
	"otui/ui"
)

const (
	Version = "v0.01.00"
	License = "Apache-2.0"
)

func main() {
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

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
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

	// Ensure cleanup on exit
	defer func() {
		if err := sessionStorage.UnlockOTUIInstance(); err != nil && config.DebugLog != nil {
			config.DebugLog.Printf("Warning: failed to unlock OTUI instance: %v", err)
		}
	}()

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

	p := tea.NewProgram(
		ui.NewAppView(cfg, sessionStorage, lastSession, Version, License),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running otui: %v\n", err)
		os.Exit(1)
	}
}
