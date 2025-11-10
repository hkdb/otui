package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type SystemConfig struct {
	DataDirectory string `toml:"data_directory"`
}

type OllamaConfig struct {
	Host         string `toml:"host"`
	DefaultModel string `toml:"default_model,omitempty"` // Kept for backward compatibility
}

// SecurityConfig defines how credentials are stored
type SecurityConfig struct {
	CredentialStorage string `toml:"credential_storage"` // "plaintext" or "ssh_key"
	SSHKeyPath        string `toml:"ssh_key_path,omitempty"`
}

// ProviderConfig defines a cloud AI provider (Anthropic, OpenRouter, etc.)
type ProviderConfig struct {
	ID      string `toml:"id"`       // e.g., "anthropic", "openrouter"
	Name    string `toml:"name"`     // Display name
	Enabled bool   `toml:"enabled"`  // Whether this provider is active
	BaseURL string `toml:"base_url"` // API base URL
	// API Key is stored separately in CredentialStore, not in config
}

type UserConfig struct {
	DefaultProvider     string           `toml:"default_provider,omitempty"`   // Which provider to use for new sessions
	DefaultModel        string           `toml:"default_model,omitempty"`      // Default model (moved from Ollama)
	LastUsedProvider    string           `toml:"last_used_provider,omitempty"` // Last provider user switched to
	Ollama              OllamaConfig     `toml:"ollama"`
	DefaultSystemPrompt string           `toml:"default_system_prompt,omitempty"`
	PluginsEnabled      bool             `toml:"plugins_enabled"`
	Security            SecurityConfig   `toml:"security"`
	Providers           []ProviderConfig `toml:"providers,omitempty"`
}

type Config struct {
	DataDirectory       string
	OllamaHost          string
	DefaultModel        string
	DefaultProvider     string // Which provider to use for new sessions
	LastUsedProvider    string // Last provider user switched to
	DefaultSystemPrompt string
	PluginsEnabled      bool
	Security            SecurityConfig
	Providers           []ProviderConfig
	CredentialStore     *CredentialStore
}

var Debug = false
var DebugLog *log.Logger

func (c *Config) OllamaURL() string {
	return c.OllamaHost
}

func (c *Config) Model() string {
	return c.DefaultModel
}

func (c *Config) DataDir() string {
	return ExpandPath(c.DataDirectory)
}

func (c *Config) applyEnvOverrides() {
	if host := os.Getenv("OTUI_OLLAMA_HOST"); host != "" {
		c.OllamaHost = host
		// Also update Providers[].BaseURL for Ollama
		for i := range c.Providers {
			if c.Providers[i].ID == "ollama" {
				c.Providers[i].BaseURL = host
				break
			}
		}
	}
	if model := os.Getenv("OTUI_OLLAMA_MODEL"); model != "" {
		c.DefaultModel = model
	}
	if dataDir := os.Getenv("OTUI_DATA_DIR"); dataDir != "" {
		c.DataDirectory = dataDir
	}
}

func CheckDebug() bool {
	debug := os.Getenv("OTUI_DEBUG")
	return debug == "true" || debug == "1"
}

func InitEarlyDebugLog() {
	if !CheckDebug() {
		return
	}

	Debug = true
	cacheDir := GetCacheDir()

	// Ensure cache dir exists
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not create cache dir for early debug log: %v\n", err)
		return
	}

	logPath := filepath.Join(cacheDir, "init-debug.log")

	// Create/append to init debug log with secure permissions (0600 - may contain sensitive debug info)
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not open init debug log at %s: %v\n", logPath, err)
		return
	}

	DebugLog = log.New(f, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	DebugLog.Printf("=== Early debug logging started (OTUI_DEBUG=%s) ===", os.Getenv("OTUI_DEBUG"))
	DebugLog.Printf("Init log path: %s", logPath)
}

func InitDebugLog(dataDir string) {
	if !CheckDebug() {
		return
	}

	// Log transition if early debug was active
	if DebugLog != nil {
		DebugLog.Printf("=== Switching from init debug log to main debug log ===")
	}

	Debug = true
	logPath := filepath.Join(dataDir, "debug.log")

	// Create debug log with secure permissions (0600 - may contain sensitive debug info)
	f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not open debug log at %s: %v\n", logPath, err)
		return
	}

	DebugLog = log.New(f, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	DebugLog.Printf("=== Main debug logging started (OTUI_DEBUG=%s) ===", os.Getenv("OTUI_DEBUG"))
	DebugLog.Printf("Log path: %s", logPath)
}

func HasAllEnvVars() bool {
	return os.Getenv("OTUI_OLLAMA_HOST") != "" &&
		os.Getenv("OTUI_OLLAMA_MODEL") != "" &&
		os.Getenv("OTUI_DATA_DIR") != ""
}

func HasAnyEnvVar() bool {
	return os.Getenv("OTUI_OLLAMA_HOST") != "" ||
		os.Getenv("OTUI_OLLAMA_MODEL") != "" ||
		os.Getenv("OTUI_DATA_DIR") != ""
}

func GetMissingEnvVar() string {
	if os.Getenv("OTUI_OLLAMA_HOST") == "" {
		return "OTUI_OLLAMA_HOST"
	}
	if os.Getenv("OTUI_OLLAMA_MODEL") == "" {
		return "OTUI_OLLAMA_MODEL"
	}
	if os.Getenv("OTUI_DATA_DIR") == "" {
		return "OTUI_DATA_DIR"
	}
	return ""
}

func Load() (*Config, error) {
	cfg := &Config{
		DataDirectory: "~/.local/share/otui",
		OllamaHost:    "http://localhost:11434",
		DefaultModel:  "llama3.1:latest",
	}

	settingsPath := GetSettingsFilePath()
	settingsExist := FileExists(settingsPath)

	if settingsExist {
		systemCfg, err := LoadSystemConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load system config: %w", err)
		}
		cfg.DataDirectory = systemCfg.DataDirectory

		dataDir := cfg.DataDir()
		userCfg, err := LoadUserConfig(dataDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load user config: %w", err)
		}
		cfg.OllamaHost = userCfg.Ollama.Host
		cfg.DefaultModel = userCfg.DefaultModel         // Read from top-level first
		cfg.DefaultProvider = userCfg.DefaultProvider   // NEW
		cfg.LastUsedProvider = userCfg.LastUsedProvider // NEW
		cfg.DefaultSystemPrompt = userCfg.DefaultSystemPrompt
		cfg.PluginsEnabled = userCfg.PluginsEnabled
		cfg.Security = userCfg.Security
		cfg.Providers = userCfg.Providers

		// MIGRATION: Move Ollama.DefaultModel to top-level if needed
		if cfg.DefaultModel == "" && userCfg.Ollama.DefaultModel != "" {
			cfg.DefaultModel = userCfg.Ollama.DefaultModel
			if Debug && DebugLog != nil {
				DebugLog.Printf("[Config] Migrated default_model from [ollama] to top-level: %s", cfg.DefaultModel)
			}
		}

		// Set default provider if not specified
		if cfg.DefaultProvider == "" {
			// Infer from enabled providers or default to "ollama"
			if len(cfg.Providers) > 0 && cfg.Providers[0].Enabled {
				cfg.DefaultProvider = cfg.Providers[0].ID
			} else {
				cfg.DefaultProvider = "ollama"
			}
			if Debug && DebugLog != nil {
				DebugLog.Printf("[Config] Set default_provider to: %s", cfg.DefaultProvider)
			}
		}

		// Set last_used_provider if not specified (same as default)
		if cfg.LastUsedProvider == "" {
			cfg.LastUsedProvider = cfg.DefaultProvider
		}
	} else if HasAllEnvVars() {
		cfg.applyEnvOverrides()
	} else {
		systemCfg, err := LoadSystemConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load system config: %w", err)
		}
		cfg.DataDirectory = systemCfg.DataDirectory

		dataDir := cfg.DataDir()
		userCfg, err := LoadUserConfig(dataDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load user config: %w", err)
		}
		cfg.OllamaHost = userCfg.Ollama.Host
		cfg.DefaultModel = userCfg.DefaultModel         // Read from top-level first
		cfg.DefaultProvider = userCfg.DefaultProvider   // NEW
		cfg.LastUsedProvider = userCfg.LastUsedProvider // NEW
		cfg.DefaultSystemPrompt = userCfg.DefaultSystemPrompt
		cfg.PluginsEnabled = userCfg.PluginsEnabled
		cfg.Security = userCfg.Security
		cfg.Providers = userCfg.Providers

		// MIGRATION: Move Ollama.DefaultModel to top-level if needed
		if cfg.DefaultModel == "" && userCfg.Ollama.DefaultModel != "" {
			cfg.DefaultModel = userCfg.Ollama.DefaultModel
			if Debug && DebugLog != nil {
				DebugLog.Printf("[Config] Migrated default_model from [ollama] to top-level: %s", cfg.DefaultModel)
			}
		}

		// Set default provider if not specified
		if cfg.DefaultProvider == "" {
			if len(cfg.Providers) > 0 && cfg.Providers[0].Enabled {
				cfg.DefaultProvider = cfg.Providers[0].ID
			} else {
				cfg.DefaultProvider = "ollama"
			}
			if Debug && DebugLog != nil {
				DebugLog.Printf("[Config] Set default_provider to: %s", cfg.DefaultProvider)
			}
		}

		// Set last_used_provider if not specified (same as default)
		if cfg.LastUsedProvider == "" {
			cfg.LastUsedProvider = cfg.DefaultProvider
		}
	}

	dataDir := cfg.DataDir()
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Ensure data directory has correct permissions (fix if needed)
	if err := EnsureDataDirPermissions(dataDir); err != nil {
		return nil, fmt.Errorf("failed to set data directory permissions: %w", err)
	}

	// Initialize CredentialStore
	// Default to plaintext if not specified
	if cfg.Security.CredentialStorage == "" {
		cfg.Security.CredentialStorage = string(SecurityPlainText)
	}

	credMethod := SecurityMethod(cfg.Security.CredentialStorage)
	cfg.CredentialStore = NewCredentialStore(credMethod, cfg.Security.SSHKeyPath)

	// Load credentials
	if err := cfg.CredentialStore.Load(dataDir); err != nil {
		// If error is due to missing passphrase, return partial config so caller can retry with passphrase
		if strings.Contains(err.Error(), "passphrase required") {
			return cfg, fmt.Errorf("failed to load credentials: %w", err)
		}
		// For other errors, return nil
		return nil, fmt.Errorf("failed to load credentials: %w", err)
	}

	return cfg, nil
}
