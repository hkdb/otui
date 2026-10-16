package model

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"otui/config"
	"otui/ollama"
)

// modelCacheTTL is how long cloud providers' model lists are cached
const modelCacheTTL = time.Hour

// getModelsFromProvider fetches a provider's models. Ollama models get their
// provider info filled in. Runs in a Cmd: must not touch the Model.
func getModelsFromProvider(providerID string, providerClient Provider) ([]ollama.ModelInfo, error) {
	models, err := providerClient.ListModels(context.Background())
	if err != nil {
		return nil, err
	}

	if providerID == "ollama" {
		// Ensure provider info is set (belt and suspenders)
		for i := range models {
			models[i].Provider = "ollama"
			if models[i].InternalName == "" {
				models[i].InternalName = models[i].Name
			}
		}
	}

	return models, nil
}

// aggregateModels fetches the models of all providers, using cached lists
// where given. Returns all models (sorted by name) and the lists that were
// fetched, for the cache. Runs in a Cmd: must not touch the Model.
func aggregateModels(providers map[string]Provider, cached map[string][]ollama.ModelInfo) ([]ollama.ModelInfo, map[string][]ollama.ModelInfo) {
	var allModels []ollama.ModelInfo
	fetched := make(map[string][]ollama.ModelInfo)

	// Include models from all providers
	for providerID, providerClient := range providers {
		models, ok := cached[providerID]
		if ok {
			if config.Debug {
				config.DebugLog.Printf("[Model] Using cached models for provider %s", providerID)
			}
		} else {
			var err error
			models, err = getModelsFromProvider(providerID, providerClient)
			if err != nil {
				// Log but don't fail - allow showing models from other providers
				if config.Debug {
					config.DebugLog.Printf("Warning: failed to fetch models from %s: %v", providerID, err)
				}
				continue
			}
			fetched[providerID] = models
		}
		allModels = append(allModels, models...)
	}

	// Sort alphabetically by display name
	sort.Slice(allModels, func(i, j int) bool {
		return allModels[i].Name < allModels[j].Name
	})

	return allModels, fetched
}

// FetchAllModels retrieves models from all enabled providers
// showSelector: whether to auto-show model selector after fetch (user-initiated vs background)
//
// Call from Update: the providers and valid cache entries are snapshotted
// here; fetched lists come back in ModelsListMsg.Fetched for StoreModelCache.
func (m *Model) FetchAllModels(showSelector bool) tea.Cmd {
	providers := make(map[string]Provider, len(m.Providers))
	cached := make(map[string][]ollama.ModelInfo)
	now := time.Now()
	for providerID, client := range m.Providers {
		providers[providerID] = client
		// Ollama: always fetch fresh (local, fast, free); cloud providers: use cache if valid
		if providerID == "ollama" {
			continue
		}
		if models, ok := m.ModelCache[providerID]; ok && now.Before(m.CacheExpiry[providerID]) {
			cached[providerID] = models
		}
	}

	return func() tea.Msg {
		models, fetched := aggregateModels(providers, cached)
		return ModelsListMsg{
			Models:       models,
			Fetched:      fetched,
			ShowSelector: showSelector,
		}
	}
}

// StoreModelCache caches model lists fetched by FetchAllModels (cloud
// providers only - Ollama is always fetched fresh). Call from Update.
func (m *Model) StoreModelCache(fetched map[string][]ollama.ModelInfo) {
	if m.ModelCache == nil {
		m.ModelCache = make(map[string][]ollama.ModelInfo)
	}
	if m.CacheExpiry == nil {
		m.CacheExpiry = make(map[string]time.Time)
	}
	for providerID, models := range fetched {
		if providerID == "ollama" {
			continue
		}
		m.ModelCache[providerID] = models
		m.CacheExpiry[providerID] = time.Now().Add(modelCacheTTL)
		if config.Debug {
			config.DebugLog.Printf("[Model] Cached %d models for provider %s", len(models), providerID)
		}
	}
}

// ClearModelCache clears the model cache for a specific provider or all providers
func (m *Model) ClearModelCache(providerID string) {
	if providerID == "" {
		// Clear all caches
		m.ModelCache = make(map[string][]ollama.ModelInfo)
		m.CacheExpiry = make(map[string]time.Time)
		if config.Debug {
			config.DebugLog.Printf("[Model] Cleared all model caches")
		}
		return
	}

	// Clear specific provider cache
	delete(m.ModelCache, providerID)
	delete(m.CacheExpiry, providerID)
	if config.Debug {
		config.DebugLog.Printf("[Model] Cleared model cache for provider %s", providerID)
	}
}

// CanSendMessage checks if the current session's provider is enabled
func (m *Model) CanSendMessage() (bool, string) {
	if m.CurrentSession == nil {
		return false, "No session loaded"
	}

	sessionProvider := m.CurrentSession.Provider
	if sessionProvider == "" {
		sessionProvider = "ollama" // Default for migrated sessions
	}

	// Check if provider exists and is enabled
	if _, ok := m.Providers[sessionProvider]; !ok {
		return false, fmt.Sprintf(
			"⚠️ This provider (%s) is disabled. You cannot send messages. "+
				"You can view your session or switch to a model with an active provider.",
			sessionProvider,
		)
	}

	return true, ""
}

// SwitchModel switches the current session to use a different model and provider.
// This is the business logic for model switching - updates session state and active provider.
//
// The method handles:
//   - Updating session.Model with InternalName (full API name like "qwen/qwen3-coder:free")
//   - Updating session.Provider to match the model's provider
//   - Switching the active m.Provider instance to the correct provider
//   - Setting the model on the new provider
//   - Marking session as dirty for auto-save
//   - Fallback handling if provider not found
//
// This encapsulates all business logic for model switching, ensuring session state
// stays consistent with the active provider.
func (m *Model) SwitchModel(modelInfo ollama.ModelInfo) tea.Cmd {
	// Update session with InternalName and Provider (business logic)
	if m.CurrentSession != nil {
		m.CurrentSession.Model = modelInfo.InternalName
		m.CurrentSession.Provider = modelInfo.Provider
		m.SessionDirty = true
	}

	// Update last_used_provider in config if changed
	if m.Config.LastUsedProvider != modelInfo.Provider {
		m.Config.LastUsedProvider = modelInfo.Provider

		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[Model] Updated last_used_provider: %s", modelInfo.Provider)
		}

		// Note: Config auto-save happens on app exit, no need to save immediately
		// This prevents blocking the UI on every model switch
	}

	// Switch to the selected provider (business logic)
	provider, ok := m.Providers[modelInfo.Provider]
	if !ok {
		// Fallback: use current provider (should not happen in normal operation)
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[Model] WARNING: Provider '%s' not found for model '%s', using fallback",
				modelInfo.Provider, modelInfo.Name)
		}
		m.Provider.SetModel(modelInfo.InternalName)
		return m.RefreshModelMetadataCmd()
	}

	m.Provider = provider
	provider.SetModel(modelInfo.InternalName)

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[Model] Switched to model '%s' (provider: %s, internal: %s)",
			modelInfo.Name, modelInfo.Provider, modelInfo.InternalName)
	}

	// Fetch the new model's real context window in the background
	return m.RefreshModelMetadataCmd()
}

// SwitchToDefaultProvider switches the active provider to the configured default.
// This is called when creating new sessions to ensure m.Provider matches the session's provider.
//
// The method handles:
//   - Looking up the default provider from m.Providers map
//   - Switching m.Provider to the default provider instance
//   - Setting the default model on the provider
//   - Fallback to current provider if default provider not found
//   - Debug logging for troubleshooting
//
// This ensures that new sessions start with the correct provider/model combination
// from the user's configuration.
func (m *Model) SwitchToDefaultProvider() {
	provider, ok := m.Providers[m.Config.DefaultProvider]
	if !ok {
		// Fallback: use current provider with config model
		if config.Debug && config.DebugLog != nil {
			config.DebugLog.Printf("[Model] WARNING: Default provider '%s' not found, using fallback",
				m.Config.DefaultProvider)
		}
		m.Provider.SetModel(m.Config.DefaultModel)
		return
	}

	m.Provider = provider
	m.Provider.SetModel(m.Config.DefaultModel)

	if config.Debug && config.DebugLog != nil {
		config.DebugLog.Printf("[Model] Switched to default provider '%s' with model '%s'",
			m.Config.DefaultProvider, m.Config.DefaultModel)
	}
}
