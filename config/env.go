package config

import (
	"fmt"
	"os"
	"strings"
)

// Environment variables for configuring OTUI without the setup wizard
// (e.g. in containers). When set, they override the profile's data
// directory, default provider/model and endpoint. Nothing is written to disk.
const (
	EnvDataDir  = "OTUI_DATA_DIR"
	EnvProvider = "OTUI_PROVIDER"
	EnvModel    = "OTUI_MODEL"
	EnvBaseURL  = "OTUI_BASE_URL"
	EnvAPIKey   = "OTUI_API_KEY"

	// Deprecated aliases (Ollama only): OTUI_PROVIDER=ollama,
	// OTUI_BASE_URL=$OTUI_OLLAMA_HOST, OTUI_MODEL=$OTUI_OLLAMA_MODEL
	EnvOllamaHost  = "OTUI_OLLAMA_HOST"
	EnvOllamaModel = "OTUI_OLLAMA_MODEL"
)

// envProviders lists the provider IDs OTUI_PROVIDER accepts
var envProviders = []string{"ollama", "openai-compatible", "openrouter", "openai", "anthropic"}

// EnvConfig is the configuration read from OTUI_* environment variables
type EnvConfig struct {
	DataDir  string
	Provider string
	Model    string
	BaseURL  string // Provider default if not set (cloud providers only)
	APIKey   string // Memory only - never stored in the CredentialStore
}

// RequiresAPIKey reports whether the env provider needs an API key
func (e *EnvConfig) RequiresAPIKey() bool {
	switch e.Provider {
	case "openrouter", "openai", "anthropic":
		return true
	}
	return false
}

// EnvUsage describes the environment variables for error messages
const EnvUsage = `Environment variables:
  • OTUI_DATA_DIR  (required) data directory
  • OTUI_PROVIDER  (required) ollama, openai-compatible,
                   openrouter, openai or anthropic
  • OTUI_MODEL     (required) default model
  • OTUI_BASE_URL  endpoint (required for ollama and
                   openai-compatible)
  • OTUI_API_KEY   API key (cloud providers)`

// ReadEnvConfig reads the OTUI_* environment variables. Returns nil, nil if
// none are set, and an error describing the problem if they're incomplete or
// invalid.
func ReadEnvConfig() (*EnvConfig, error) {
	env := &EnvConfig{
		DataDir:  strings.TrimSpace(os.Getenv(EnvDataDir)),
		Provider: strings.TrimSpace(os.Getenv(EnvProvider)),
		Model:    strings.TrimSpace(os.Getenv(EnvModel)),
		BaseURL:  strings.TrimSpace(os.Getenv(EnvBaseURL)),
		APIKey:   strings.TrimSpace(os.Getenv(EnvAPIKey)),
	}
	ollamaHost := strings.TrimSpace(os.Getenv(EnvOllamaHost))
	ollamaModel := strings.TrimSpace(os.Getenv(EnvOllamaModel))

	if *env == (EnvConfig{}) && ollamaHost == "" && ollamaModel == "" {
		return nil, nil
	}

	// Deprecated Ollama aliases (the new variables take precedence)
	if env.Provider == "" && (ollamaHost != "" || ollamaModel != "") {
		env.Provider = "ollama"
	}
	if env.Provider == "ollama" {
		if env.BaseURL == "" {
			env.BaseURL = ollamaHost
		}
		if env.Model == "" {
			env.Model = ollamaModel
		}
	}

	var missing []string
	if env.DataDir == "" {
		missing = append(missing, EnvDataDir)
	}
	if env.Provider == "" {
		missing = append(missing, EnvProvider)
	}
	if env.Model == "" {
		missing = append(missing, EnvModel)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing environment variable(s): %s", strings.Join(missing, ", "))
	}

	known := false
	for _, id := range envProviders {
		if env.Provider == id {
			known = true
			break
		}
	}
	if !known {
		return nil, fmt.Errorf("unknown %s %q (expected one of: %s)",
			EnvProvider, env.Provider, strings.Join(envProviders, ", "))
	}

	if env.BaseURL == "" {
		if env.Provider == "ollama" || env.Provider == "openai-compatible" {
			return nil, fmt.Errorf("%s is required for %s", EnvBaseURL, env.Provider)
		}
		env.BaseURL = getProviderDefaultBaseURL(env.Provider)
	}

	return env, nil
}
