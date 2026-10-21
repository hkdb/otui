package model

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"otui/config"
)

// modelMetadataTimeout bounds a metadata refresh (all of a provider's probes)
const modelMetadataTimeout = 10 * time.Second

// defaultModelMetadata is used when nothing is known about a model
var defaultModelMetadata = ModelMetadata{
	ContextWindow: 8192,
	MaxOutput:     2048,
	SupportsTools: false,
	Source:        MetadataSourceFallback,
}

// ModelMetadataMsg carries metadata fetched by RefreshModelMetadataCmd
type ModelMetadataMsg struct {
	ProviderID string
	ModelName  string
	Metadata   ModelMetadata
	Err        error
}

func metadataCacheKey(providerID, modelName string) string {
	return providerID + "\x00" + modelName
}

// sessionProvider returns the provider ID and instance for the current
// session's model. Uses the session's own provider rather than m.Provider,
// which can point at a different provider (e.g. the default one).
func (m *Model) sessionProvider() (string, Provider) {
	if m.CurrentSession == nil {
		return "", m.Provider
	}

	providerID := m.CurrentSession.Provider
	if providerID == "" {
		providerID = "ollama" // Sessions from before multi-provider support
	}
	if p, ok := m.Providers[providerID]; ok {
		return providerID, p
	}
	return providerID, m.Provider
}

// GetModelMetadata returns metadata for the current session's model.
// Checks user overrides first, then metadata fetched from the provider, then
// the provider's hardcoded fallback. Never does network I/O, so it's safe to
// call from View.
func (m *Model) GetModelMetadata() ModelMetadata {
	if m.CurrentSession == nil {
		return defaultModelMetadata
	}

	modelName := m.CurrentSession.Model

	// 1. User config overrides (highest priority)
	if contextWindow, ok := m.Config.ModelContextOverrides[modelName]; ok {
		return ModelMetadata{
			ContextWindow: contextWindow,
			MaxOutput:     8192, // Reasonable default
			SupportsTools: true, // Assume yes if user is overriding
			Source:        "override",
		}
	}

	providerID, provider := m.sessionProvider()

	// 2. Metadata fetched from the provider
	m.metadataMu.RLock()
	meta, ok := m.metadataCache[metadataCacheKey(providerID, modelName)]
	m.metadataMu.RUnlock()
	if ok {
		return meta
	}

	// 3. Hardcoded fallback until the fetch completes
	if provider != nil {
		return provider.FallbackModelMetadata(modelName)
	}
	return defaultModelMetadata
}

// RefreshModelMetadataCmd fetches the current session model's metadata from
// its provider in the background. The result is stored by SetModelMetadata.
//
// Refresh whenever the effective context may have changed: on startup,
// session/model switch, provider settings change and after each response
// (e.g. Ollama only reports the context a model runs with once it's loaded).
func (m *Model) RefreshModelMetadataCmd() tea.Cmd {
	if m.CurrentSession == nil {
		return nil
	}

	modelName := m.CurrentSession.Model
	if modelName == "" {
		return nil
	}
	if _, ok := m.Config.ModelContextOverrides[modelName]; ok {
		return nil // Override wins, nothing to fetch
	}

	providerID, provider := m.sessionProvider()
	if provider == nil {
		return nil
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), modelMetadataTimeout)
		defer cancel()

		meta, err := provider.GetModelMetadata(ctx, modelName)
		return ModelMetadataMsg{
			ProviderID: providerID,
			ModelName:  modelName,
			Metadata:   meta,
			Err:        err,
		}
	}
}

// SetModelMetadata stores metadata fetched by RefreshModelMetadataCmd.
// Must be called from Update.
func (m *Model) SetModelMetadata(msg ModelMetadataMsg) {
	if msg.Err != nil || msg.Metadata.ContextWindow <= 0 {
		config.Debugf("[metadata] Fetch failed for %s/%s: %v", msg.ProviderID, msg.ModelName, msg.Err)
		return
	}

	m.metadataMu.Lock()
	if m.metadataCache == nil {
		m.metadataCache = make(map[string]ModelMetadata)
	}
	m.metadataCache[metadataCacheKey(msg.ProviderID, msg.ModelName)] = msg.Metadata
	m.metadataMu.Unlock()

	config.Debugf("[metadata] %s/%s: context window %d (source: %s)",
		msg.ProviderID, msg.ModelName, msg.Metadata.ContextWindow, msg.Metadata.Source)
}

// ClearModelMetadata drops all fetched metadata, e.g. after provider settings
// change (a different base URL can serve a different context window)
func (m *Model) ClearModelMetadata() {
	m.metadataMu.Lock()
	m.metadataCache = nil
	m.metadataMu.Unlock()
}
