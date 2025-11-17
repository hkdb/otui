package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type PluginConfigEntry struct {
	Enabled       bool              `toml:"enabled"`
	Config        map[string]string `toml:"config,omitempty"`         // Non-sensitive OR all values if plaintext
	SensitiveKeys []string          `toml:"sensitive_keys,omitempty"` // Keys stored in CredentialStore
}

type PluginsConfig struct {
	Plugins map[string]PluginConfigEntry `toml:"plugins"`
}

func LoadPluginsConfig(dataDir string) (*PluginsConfig, error) {
	pluginsConfigPath := filepath.Join(dataDir, "plugins.toml")

	if _, err := os.Stat(pluginsConfigPath); os.IsNotExist(err) {
		return &PluginsConfig{
			Plugins: make(map[string]PluginConfigEntry),
		}, nil
	}

	var config PluginsConfig
	if _, err := toml.DecodeFile(pluginsConfigPath, &config); err != nil {
		return nil, fmt.Errorf("failed to decode plugins config: %w", err)
	}

	if config.Plugins == nil {
		config.Plugins = make(map[string]PluginConfigEntry)
	}

	return &config, nil
}

func SavePluginsConfig(dataDir string, config *PluginsConfig) error {
	pluginsConfigPath := filepath.Join(dataDir, "plugins.toml")

	// Data dir should already exist with correct perms from config.Load()
	// But ensure it exists just in case (0700 - user-only access)
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create file with secure permissions (0600 - will contain API keys in future)
	f, err := os.OpenFile(pluginsConfigPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create plugins config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode plugins config: %w", err)
	}

	return nil
}

func (pc *PluginsConfig) GetPluginEnabled(pluginID string) bool {
	entry, exists := pc.Plugins[pluginID]
	if !exists {
		return false
	}
	return entry.Enabled
}

func (pc *PluginsConfig) SetPluginEnabled(pluginID string, enabled bool) {
	if pc.Plugins == nil {
		pc.Plugins = make(map[string]PluginConfigEntry)
	}

	entry, exists := pc.Plugins[pluginID]
	if !exists {
		entry = PluginConfigEntry{
			Config: make(map[string]string),
		}
	}

	entry.Enabled = enabled
	pc.Plugins[pluginID] = entry
}

func (pc *PluginsConfig) GetPluginConfig(pluginID string) map[string]string {
	entry, exists := pc.Plugins[pluginID]
	if !exists {
		return make(map[string]string)
	}

	if entry.Config == nil {
		return make(map[string]string)
	}

	return entry.Config
}

func (pc *PluginsConfig) SetPluginConfig(pluginID string, config map[string]string) {
	if pc.Plugins == nil {
		pc.Plugins = make(map[string]PluginConfigEntry)
	}

	entry, exists := pc.Plugins[pluginID]
	if !exists {
		entry = PluginConfigEntry{
			Enabled: false,
		}
	}

	entry.Config = config
	pc.Plugins[pluginID] = entry
}

func (pc *PluginsConfig) DeletePlugin(pluginID string) {
	if pc.Plugins == nil {
		return
	}
	delete(pc.Plugins, pluginID)
}

// isSensitiveKey determines if a key contains sensitive data
func isSensitiveKey(key string) bool {
	upperKey := strings.ToUpper(key)
	sensitiveWords := []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "AUTH", "CREDENTIAL", "BEARER"}
	for _, word := range sensitiveWords {
		switch {
		case strings.Contains(upperKey, word):
			return true
		}
	}
	return false
}

// SavePluginConfigSecure saves plugin config with proper security handling
func SavePluginConfigSecure(cfg *Config, dataDir string, pluginsConfig *PluginsConfig, pluginID string, configValues map[string]string) error {
	switch cfg.Security.CredentialStorage {
	case string(SecuritySSHKey):
		// Separate sensitive from non-sensitive
		sensitiveKeys := []string{}
		plaintextConfig := make(map[string]string)

		for key, value := range configValues {
			isSensitive := isSensitiveKey(key)
			switch {
			case isSensitive:
				// Store in CredentialStore (encrypted)
				if err := cfg.CredentialStore.SetPlugin(pluginID, key, value); err != nil {
					return fmt.Errorf("failed to save sensitive key %s: %w", key, err)
				}
				sensitiveKeys = append(sensitiveKeys, key)
			default:
				// Store in plaintext TOML
				plaintextConfig[key] = value
			}
		}

		// Save config entry
		entry := PluginConfigEntry{
			Enabled:       pluginsConfig.GetPluginEnabled(pluginID),
			Config:        plaintextConfig,
			SensitiveKeys: sensitiveKeys,
		}
		pluginsConfig.Plugins[pluginID] = entry

		// Save encrypted credentials
		return cfg.CredentialStore.Save(dataDir)

	case string(SecurityPlainText):
		// Store everything in plaintext
		entry := PluginConfigEntry{
			Enabled:       pluginsConfig.GetPluginEnabled(pluginID),
			Config:        configValues,
			SensitiveKeys: []string{}, // Empty - using plaintext
		}
		pluginsConfig.Plugins[pluginID] = entry
		return nil

	default:
		return fmt.Errorf("unknown security method: %s", cfg.Security.CredentialStorage)
	}
}

// LoadPluginConfigSecure loads plugin config with proper security handling
func LoadPluginConfigSecure(cfg *Config, pluginsConfig *PluginsConfig, pluginID string) (map[string]string, error) {
	entry, exists := pluginsConfig.Plugins[pluginID]
	switch {
	case !exists:
		return make(map[string]string), nil
	}

	result := make(map[string]string)

	// Load plaintext config
	for key, value := range entry.Config {
		result[key] = value
	}

	// Load sensitive keys from CredentialStore (only if using encryption)
	switch cfg.Security.CredentialStorage {
	case string(SecuritySSHKey):
		for _, key := range entry.SensitiveKeys {
			value := cfg.CredentialStore.GetPlugin(pluginID, key)
			switch {
			case value == "":
				continue
			}
			result[key] = value
		}
	case string(SecurityPlainText):
		// All values already in Config, nothing more to load
	default:
		return nil, fmt.Errorf("unknown security method: %s", cfg.Security.CredentialStorage)
	}

	return result, nil
}
