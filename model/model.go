package model

import (
	"otui/config"
	"otui/mcp"
	"otui/ollama"
	"otui/storage"
)

// Model holds the core application data and business logic state
type Model struct {
	// Core dependencies
	Config         *config.Config
	OllamaClient   *ollama.Client
	SessionStorage *storage.SessionStorage
	MCPManager     *mcp.MCPManager

	// Application data
	Messages       []Message
	CurrentSession *storage.Session
	SearchIndex    *storage.SearchIndex
	Plugins        *PluginState

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
func NewModel(cfg *config.Config, sessionStorage *storage.SessionStorage, lastSession *storage.Session, plugins *PluginState, mcpManager *mcp.MCPManager, searchIndex *storage.SearchIndex, version, license string) *Model {
	// Initialize Ollama client (allow nil for offline mode - Phase 4)
	client, err := ollama.NewClient(cfg.OllamaURL(), cfg.Model())
	if err != nil {
		// Don't panic - allow offline mode
		if config.DebugLog != nil {
			config.DebugLog.Printf("[Model] Ollama client creation failed: %v (running in offline mode)", err)
		}
		client = nil
	}

	// Set model from last session if available
	if client != nil && lastSession != nil && lastSession.Model != "" {
		client.SetModel(lastSession.Model)
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
		OllamaClient:       client,
		SessionStorage:     sessionStorage,
		MCPManager:         mcpManager,
		Messages:           messages,
		CurrentSession:     lastSession,
		SearchIndex:        searchIndex,
		Plugins:            plugins,
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
