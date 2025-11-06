package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type PluginConfigEntry struct {
	Enabled bool              `toml:"enabled"`
	Config  map[string]string `toml:"config,omitempty"`
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
