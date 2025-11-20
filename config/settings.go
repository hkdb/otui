package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

func LoadSystemConfig() (*SystemConfig, error) {
	cfg := DefaultSystemConfig()
	settingsPath := GetSettingsFilePath()

	if !FileExists(settingsPath) {
		if err := CreateDefaultSystemConfig(); err != nil {
			return nil, fmt.Errorf("failed to create system config: %w", err)
		}
		return cfg, nil
	}

	_, err := toml.DecodeFile(settingsPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse system config: %w", err)
	}

	return cfg, nil
}

// SystemConfigExists checks if the system config file exists
// without creating it (unlike LoadSystemConfig which creates if missing)
func SystemConfigExists() bool {
	settingsPath := GetSettingsFilePath()
	return FileExists(settingsPath)
}

func LoadUserConfig(dataDir string) (*UserConfig, error) {
	cfg := DefaultUserConfig()
	userConfigPath := filepath.Join(dataDir, "config.toml")

	if !FileExists(userConfigPath) {
		if err := CreateDefaultUserConfig(dataDir); err != nil {
			return nil, fmt.Errorf("failed to create user config: %w", err)
		}
		return cfg, nil
	}

	// Read file content for migration check
	content, err := os.ReadFile(userConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read user config: %w", err)
	}

	_, err = toml.DecodeFile(userConfigPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	// MIGRATION: Add enable_multi_step if not present (Phase 2 feature)
	if !strings.Contains(string(content), "enable_multi_step") {
		// Append to config file
		migration := "\n# Multi-step execution (Phase 2)\nenable_multi_step = true\nmax_iterations = 10\n"
		f, err := os.OpenFile(userConfigPath, os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			f.WriteString(migration)
			f.Close()
			if Debug && DebugLog != nil {
				DebugLog.Printf("[Config] Migrated: added enable_multi_step and max_iterations to config")
			}
		}
		// Set in memory (since we already decoded before migration)
		cfg.EnableMultiStep = true
		cfg.MaxIterations = 10
	}

	return cfg, nil
}

// LoadUserConfigFromPath loads user config from a specific file path
// Returns nil if the file doesn't exist (not an error)
func LoadUserConfigFromPath(configPath string) (*UserConfig, error) {
	if !FileExists(configPath) {
		return nil, nil
	}

	cfg := &UserConfig{}
	_, err := toml.DecodeFile(configPath, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}

	return cfg, nil
}

func SaveSystemConfig(cfg *SystemConfig) error {
	configDir := GetConfigDir()
	if err := EnsureDir(configDir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	settingsPath := GetSettingsFilePath()
	// Create with secure permissions (0600 - contains Ollama host/model settings)
	f, err := os.OpenFile(settingsPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create system config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode system config: %w", err)
	}

	return nil
}

func SaveUserConfig(cfg *UserConfig, dataDir string) error {
	// Data dir should already exist with correct perms (0700)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	userConfigPath := filepath.Join(dataDir, "config.toml")
	// Create with secure permissions (0600 - user configuration data)
	f, err := os.OpenFile(userConfigPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create user config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(cfg); err != nil {
		return fmt.Errorf("failed to encode user config: %w", err)
	}

	return nil
}

func CreateDefaultSystemConfig() error {
	configDir := GetConfigDir()
	if err := EnsureDir(configDir); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	settingsPath := GetSettingsFilePath()
	if FileExists(settingsPath) {
		return nil
	}

	content := GenerateSystemConfigTemplate()
	if err := os.WriteFile(settingsPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write system config: %w", err)
	}

	return nil
}

func CreateDefaultUserConfig(dataDir string) error {
	// Data dir should already exist with correct perms (0700)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	userConfigPath := filepath.Join(dataDir, "config.toml")
	if FileExists(userConfigPath) {
		return nil
	}

	content := GenerateUserConfigTemplate()
	if err := os.WriteFile(userConfigPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write user config: %w", err)
	}

	return nil
}
