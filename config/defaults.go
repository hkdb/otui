package config

func DefaultSystemConfig() *SystemConfig {
	return &SystemConfig{
		DataDirectory: "~/.local/share/otui",
	}
}

func DefaultUserConfig() *UserConfig {
	return &UserConfig{
		Ollama: OllamaConfig{
			Host:         "http://localhost:11434",
			DefaultModel: "llama3.1:latest",
		},
		PluginsEnabled: false,
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

[ollama]
# Ollama server URL
host = "http://localhost:11434"

# Default model to use when starting a new session
default_model = "llama3.1:latest"

# Default system prompt for new sessions (optional)
# Example: "You are a helpful coding assistant."
default_system_prompt = ""

# Plugin System (disabled by default)
# Enable to use MCP plugins for extended tool capabilities
plugins_enabled = false
`
}
