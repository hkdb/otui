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
		Providers: []ProviderConfig{},
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
