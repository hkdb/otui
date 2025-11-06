package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

type SystemConfig struct {
	DataDirectory string `toml:"data_directory"`
}

type OllamaConfig struct {
	Host         string `toml:"host"`
	DefaultModel string `toml:"default_model"`
}

type UserConfig struct {
	Ollama              OllamaConfig `toml:"ollama"`
	DefaultSystemPrompt string       `toml:"default_system_prompt,omitempty"`
	PluginsEnabled      bool         `toml:"plugins_enabled"`
}

type Config struct {
	DataDirectory       string
	OllamaHost          string
	DefaultModel        string
	DefaultSystemPrompt string
	PluginsEnabled      bool
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

func InitDebugLog(dataDir string) {
	if !CheckDebug() {
		return
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
	DebugLog.Printf("=== Debug logging started (OTUI_DEBUG=%s) ===", os.Getenv("OTUI_DEBUG"))
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
		cfg.DefaultModel = userCfg.Ollama.DefaultModel
		cfg.DefaultSystemPrompt = userCfg.DefaultSystemPrompt
		cfg.PluginsEnabled = userCfg.PluginsEnabled
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
		cfg.DefaultModel = userCfg.Ollama.DefaultModel
		cfg.DefaultSystemPrompt = userCfg.DefaultSystemPrompt
		cfg.PluginsEnabled = userCfg.PluginsEnabled
	}

	dataDir := cfg.DataDir()
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Ensure data directory has correct permissions (fix if needed)
	if err := EnsureDataDirPermissions(dataDir); err != nil {
		return nil, fmt.Errorf("failed to set data directory permissions: %w", err)
	}

	return cfg, nil
}
