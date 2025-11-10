package model

import (
	"time"

	"otui/config"
	"otui/mcp"
	"otui/ollama"
	"otui/storage"
)

// Model holds the core application data and business logic state
type Model struct {
	// Core dependencies
	Config         *config.Config
	Provider       Provider            // Current session's provider
	Providers      map[string]Provider // All enabled providers (map[provider_id]Provider)
	SessionStorage *storage.SessionStorage
	MCPManager     *mcp.MCPManager

	// Application data
	Messages       []Message
	CurrentSession *storage.Session
	SearchIndex    *storage.SearchIndex
	Plugins        *PluginState

	// Model caching (for cloud providers)
	ModelCache  map[string][]ollama.ModelInfo // Cached models per provider
	CacheExpiry map[string]time.Time          // Cache expiry per provider

	// Runtime state (not UI)
	Streaming          bool
	SessionDirty       bool
	NeedsInitialRender bool
	Quitting           bool

	// Application metadata
	Version string
	License string
}

// NewModel creates a new Model with the given configuration
func NewModel(cfg *config.Config, providerClient Provider, sessionStorage *storage.SessionStorage, lastSession *storage.Session, plugins *PluginState, mcpManager *mcp.MCPManager, searchIndex *storage.SearchIndex, version, license string) *Model {
	// Set model from last session if available
	if providerClient != nil && lastSession != nil && lastSession.Model != "" {
		providerClient.SetModel(lastSession.Model)
	}

	// Load messages from last session if available
	var messages []Message
	needsRender := false
	if lastSession != nil {
		for _, sMsg := range lastSession.Messages {
			messages = append(messages, Message{
				Role:      sMsg.Role,
				Content:   sMsg.Content,
				Rendered:  sMsg.Rendered,
				Timestamp: sMsg.Timestamp,
			})
		}
		needsRender = len(messages) > 0
	}

	m := &Model{
		Config:             cfg,
		Provider:           providerClient,
		Providers:          make(map[string]Provider),
		SessionStorage:     sessionStorage,
		MCPManager:         mcpManager,
		Messages:           messages,
		CurrentSession:     lastSession,
		SearchIndex:        searchIndex,
		Plugins:            plugins,
		ModelCache:         make(map[string][]ollama.ModelInfo),
		CacheExpiry:        make(map[string]time.Time),
		Streaming:          false,
		SessionDirty:       false,
		NeedsInitialRender: needsRender,
		Quitting:           false,
		Version:            version,
		License:            license,
	}

	// Sync session with MCP Manager if both exist
	// This ensures auto-loaded sessions (on app startup) have tool context
	// Safe to call even if plugins disabled - GetTools() has guards
	if mcpManager != nil && lastSession != nil {
		_ = mcpManager.SetSession(lastSession)
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Model] NewModel: Synced session '%s' with MCP Manager (EnabledPlugins: %v)",
				lastSession.Name, lastSession.EnabledPlugins)
		}
	}

	return m
}
