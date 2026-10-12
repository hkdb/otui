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

// CompactionConfig defines settings for context window management
type CompactionConfig struct {
	AutoCompact          bool    `toml:"auto_compact"`           // Whether to auto-compact when threshold reached
	AutoCompactThreshold float64 `toml:"auto_compact_threshold"` // Percentage (0.0-1.0) at which to trigger auto-compact
	KeepPercentage       float64 `toml:"keep_percentage"`        // Percentage (0.0-1.0) of context to keep when compacting
	WarnAtPercentage     float64 `toml:"warn_at_percentage"`     // Percentage (0.0-1.0) at which to show warning
}

type UserConfig struct {
	DefaultProvider        string           `toml:"default_provider,omitempty"`   // Which provider to use for new sessions
	DefaultModel           string           `toml:"default_model,omitempty"`      // Default model (moved from Ollama)
	LastUsedProvider       string           `toml:"last_used_provider,omitempty"` // Last provider user switched to
	Ollama                 OllamaConfig     `toml:"ollama"`
	DefaultSystemPrompt    string           `toml:"default_system_prompt,omitempty"`
	PluginsEnabled         bool             `toml:"plugins_enabled"`
	Security               SecurityConfig   `toml:"security"`
	Providers              []ProviderConfig `toml:"providers,omitempty"`
	AllowedTools           []string         `toml:"allowed_tools,omitempty"` // Global whitelist of tools that don't require approval
	RequireApproval        bool             `toml:"require_approval"`        // Whether to ask for permission before executing tools
	MaxIterations          int              `toml:"max_iterations"`          // Default: 10
	EnableMultiStep        bool             `toml:"enable_multi_step"`       // Default: true
	NotifyOnComplete       bool             `toml:"notify_on_complete"`      // Emit terminal bell when LLM response completes
	Compaction             CompactionConfig `toml:"compaction,omitempty"`    // Context window management settings
	ModelContextOverrides  map[string]int   `toml:"model_context_overrides,omitempty"` // Per-model context window overrides
}

type Config struct {
	DataDirectory         string
	OllamaHost            string
	DefaultModel          string
	DefaultProvider       string // Which provider to use for new sessions
	LastUsedProvider      string // Last provider user switched to
	DefaultSystemPrompt   string
	PluginsEnabled        bool
	Security              SecurityConfig
	Providers             []ProviderConfig
	CredentialStore       *CredentialStore
	AllowedTools          []string // Global whitelist of tools that don't require approval
	RequireApproval       bool     // Whether to ask for permission before executing tools
	MaxIterations         int      // Max iterations per user message
	EnableMultiStep       bool     // Allow LLM to execute multiple steps
	NotifyOnComplete      bool     // Emit terminal bell when LLM response completes
	Compaction            CompactionConfig // Context window management settings
	ModelContextOverrides map[string]int   // Per-model context window overrides
	Keybindings           *KeyBindingsConfig
	Env                   *EnvConfig // Set when configured through OTUI_* environment variables
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

// applyUserConfig copies the profile's config.toml settings and fills in
// defaults and migrations
func (c *Config) applyUserConfig(userCfg *UserConfig) {
	c.OllamaHost = userCfg.Ollama.Host
	c.DefaultModel = userCfg.DefaultModel // Read from top-level first
	c.DefaultProvider = userCfg.DefaultProvider
	c.LastUsedProvider = userCfg.LastUsedProvider
	c.DefaultSystemPrompt = userCfg.DefaultSystemPrompt
	c.PluginsEnabled = userCfg.PluginsEnabled
	c.Security = userCfg.Security
	c.Providers = userCfg.Providers
	c.AllowedTools = userCfg.AllowedTools
	c.RequireApproval = userCfg.RequireApproval
	c.MaxIterations = userCfg.MaxIterations
	c.EnableMultiStep = userCfg.EnableMultiStep
	c.NotifyOnComplete = userCfg.NotifyOnComplete
	c.Compaction = userCfg.Compaction
	c.ModelContextOverrides = userCfg.ModelContextOverrides

	// Set defaults for multi-step execution (Phase 2)
	if c.MaxIterations == 0 {
		c.MaxIterations = 10
	}

	// Set defaults for compaction if not specified
	if c.Compaction.AutoCompactThreshold == 0 {
		c.Compaction.AutoCompactThreshold = 0.75
	}
	if c.Compaction.KeepPercentage == 0 {
		c.Compaction.KeepPercentage = 0.50
	}
	if c.Compaction.WarnAtPercentage == 0 {
		c.Compaction.WarnAtPercentage = 0.85
	}

	// MIGRATION: Move Ollama.DefaultModel to top-level if needed
	if c.DefaultModel == "" && userCfg.Ollama.DefaultModel != "" {
		c.DefaultModel = userCfg.Ollama.DefaultModel
		if Debug && DebugLog != nil {
			DebugLog.Printf("[Config] Migrated default_model from [ollama] to top-level: %s", c.DefaultModel)
		}
	}

	// Set default provider if not specified
	if c.DefaultProvider == "" {
		// Infer from enabled providers or default to "ollama"
		if len(c.Providers) > 0 && c.Providers[0].Enabled {
			c.DefaultProvider = c.Providers[0].ID
		}
		if c.DefaultProvider == "" {
			c.DefaultProvider = "ollama"
		}
		if Debug && DebugLog != nil {
			DebugLog.Printf("[Config] Set default_provider to: %s", c.DefaultProvider)
		}
	}

	// Set last_used_provider if not specified (same as default)
	if c.LastUsedProvider == "" {
		c.LastUsedProvider = c.DefaultProvider
	}
}

// applyEnvConfig makes the env provider/model/endpoint the defaults.
// Only changes the in-memory config.
func (c *Config) applyEnvConfig(env *EnvConfig) {
	c.DataDirectory = env.DataDir
	c.DefaultProvider = env.Provider
	c.LastUsedProvider = env.Provider
	c.DefaultModel = env.Model

	if env.Provider == "ollama" {
		c.OllamaHost = env.BaseURL
	}

	// Copy so the profile's provider list isn't shared with UserConfig
	providers := append([]ProviderConfig(nil), c.Providers...)
	found := false
	for i := range providers {
		if providers[i].ID == env.Provider {
			providers[i].Enabled = true
			providers[i].BaseURL = env.BaseURL
			found = true
			break
		}
	}
	if !found {
		providers = append(providers, ProviderConfig{
			ID:      env.Provider,
			Name:    getProviderDisplayName(env.Provider),
			Enabled: true,
			BaseURL: env.BaseURL,
		})
	}
	c.Providers = providers
}

// APIKey returns the API key for a provider: OTUI_API_KEY for the env
// provider if set, otherwise the stored credential. The env key is never
// put into the CredentialStore, so it can't end up on disk.
func (c *Config) APIKey(providerID string) string {
	if c.Env != nil && c.Env.Provider == providerID && c.Env.APIKey != "" {
		return c.Env.APIKey
	}
	if c.CredentialStore == nil {
		return ""
	}
	return c.CredentialStore.Get(providerID)
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

func Load() (*Config, error) {
	cfg := &Config{
		DataDirectory: "~/.local/share/otui",
		OllamaHost:    "http://localhost:11434",
		DefaultModel:  "llama3.1:latest",
	}

	// Environment variables (e.g. in containers) override the profile
	env, err := ReadEnvConfig()
	if err != nil {
		return nil, err
	}
	cfg.Env = env

	// Data directory: OTUI_DATA_DIR, else settings.toml (created if missing)
	if env != nil {
		cfg.DataDirectory = env.DataDir
	} else {
		systemCfg, err := LoadSystemConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to load system config: %w", err)
		}
		cfg.DataDirectory = systemCfg.DataDirectory
	}

	userCfg, err := LoadUserConfig(cfg.DataDir())
	if err != nil {
		return nil, fmt.Errorf("failed to load user config: %w", err)
	}
	cfg.applyUserConfig(userCfg)

	// Env values win over the profile (in memory only, never saved)
	if env != nil {
		cfg.applyEnvConfig(env)
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

	// Load keybindings before credentials so the partial config returned when
	// a passphrase is required is complete apart from credentials
	keybindingsCfg, err := LoadKeybindings(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load keybindings: %w", err)
	}
	cfg.Keybindings = keybindingsCfg

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
