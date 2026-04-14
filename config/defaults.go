package config

func DefaultSystemConfig() *SystemConfig {
	return &SystemConfig{
		DataDirectory: "~/.local/share/otui",
	}
}

func DefaultUserConfig() *UserConfig {
	return &UserConfig{
		DefaultProvider:  "ollama",          // Default provider for new sessions
		DefaultModel:     "llama3.1:latest", // Default model (top-level)
		LastUsedProvider: "ollama",          // Last used provider
		Ollama: OllamaConfig{
			Host: "http://localhost:11434",
			// DefaultModel removed (migrated to top-level)
		},
		PluginsEnabled: false,
		Security: SecurityConfig{
			CredentialStorage: string(SecurityPlainText),
			SSHKeyPath:        "",
		},
		Providers:       []ProviderConfig{},
		AllowedTools:    []string{},
		RequireApproval: true, // Default to requiring approval for safety
		MaxIterations:   10,   // Default max iterations per user message
		EnableMultiStep:  true,  // Default to allowing multi-step execution
		NotifyOnComplete: false, // No notification by default
		Compaction: CompactionConfig{
			AutoCompact:          false, // Manual compaction by default
			AutoCompactThreshold: 0.75,  // Trigger at 75% context usage
			KeepPercentage:       0.50,  // Keep last 50% when compacting
			WarnAtPercentage:     0.85,  // Show warning at 85% usage
		},
		ModelContextOverrides: make(map[string]int), // Empty by default
	}
}

func GenerateSystemConfigTemplate() string {
	return `# OTUI System Configuration
# Location: ~/.config/otui/settings.toml
# This file uses TOML format: https://toml.io

# Directory where sessions and user config are stored
data_directory = "~/.local/share/otui"
`
}

func GenerateUserConfigTemplate() string {
	return `# OTUI User Configuration
# Location: <data_directory>/config.toml
# This file uses TOML format: https://toml.io

# Multi-Provider Defaults
# Which provider to use for new sessions ("ollama", "openrouter", "anthropic", etc.)
default_provider = "ollama"

# Default model to use when starting a new session (InternalName format)
# Examples: "llama3.1:latest" (ollama), "qwen/qwen3-coder:free" (openrouter)
default_model = "llama3.1:latest"

# Last used provider (automatically updated when switching models)
last_used_provider = "ollama"

[ollama]
# Ollama server URL
host = "http://localhost:11434"

# Default system prompt for new sessions (optional)
default_system_prompt = ""

# Plugin System
plugins_enabled = false

# Tool Permissions
# Require user approval before executing tools (default: true for safety)
require_approval = true

# Global whitelist of tools that don't require approval (e.g., ["filesystem.read_file"])
# allowed_tools = []

# Multi-step execution (Phase 2)
enable_multi_step = true  # Allow LLM to execute multiple steps in sequence
max_iterations = 10       # Maximum steps per user message

# Desktop Notification
# Emit terminal bell when LLM response completes
# Works in native, Flatpak, and Docker containers
# Configure your terminal emulator to show visual notifications on bell
notify_on_complete = false

# Context Window Management
[compaction]
auto_compact = false            # Automatically compact when context usage exceeds threshold
auto_compact_threshold = 0.75   # Trigger auto-compact at 75% context usage (0.0-1.0)
keep_percentage = 0.50          # Keep last 50% of context when compacting (0.0-1.0)
warn_at_percentage = 0.85       # Show warning indicator at 85% usage (0.0-1.0)

# Per-Model Context Window Overrides (optional)
# Override context window size for specific models (in tokens)
# [model_context_overrides]
# "custom-model:latest" = 64000
# "qwen3-coder:70b" = 131072

[security]
credential_storage = "plaintext"
# ssh_key_path = "~/.ssh/otui_ed25519"

# Cloud AI Providers (optional)
# [[providers]]
# id = "openrouter"
# name = "OpenRouter"
# enabled = true
# base_url = "https://openrouter.ai/api/v1"
`
}
