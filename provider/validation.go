package provider

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"otui/config"
	"otui/ollama"
)

// PingProviderMsg is sent when provider ping completes
type PingProviderMsg struct {
	ProviderID string
	Valid      bool
	Err        error
}

// SingleProviderModelsMsg is sent when models fetched from single provider
type SingleProviderModelsMsg struct {
	ProviderID string
	Models     []ollama.ModelInfo
	Err        error
}

// PingProvider validates a provider's credentials by calling Ping()
// Used during wizard setup to validate API keys before fetching models
func PingProvider(providerID, baseURL, apiKey string) tea.Cmd {
	return func() tea.Msg {
		providerType := MapProviderIDToType(providerID)

		p, err := NewProvider(Config{
			Type:    providerType,
			BaseURL: baseURL,
			APIKey:  apiKey,
			Model:   "",
		})

		if err != nil {
			return PingProviderMsg{
				ProviderID: providerID,
				Valid:      false,
				Err:        fmt.Errorf("failed to create provider: %w", err),
			}
		}

		ctx := context.Background()
		if err := p.Ping(ctx); err != nil {
			return PingProviderMsg{
				ProviderID: providerID,
				Valid:      false,
				Err:        fmt.Errorf("connection failed: %w", err),
			}
		}

		if config.Debug {
			config.DebugLog.Printf("[Provider] Provider %s ping successful", providerID)
		}

		return PingProviderMsg{
			ProviderID: providerID,
			Valid:      true,
			Err:        nil,
		}
	}
}

// FetchSingleProviderModels fetches models from a specific provider
// Used during wizard setup to fetch models from each configured provider
func FetchSingleProviderModels(providerID, baseURL, apiKey, ollamaURL string) tea.Cmd {
	return func() tea.Msg {
		var models []ollama.ModelInfo

		switch providerID {
		case "ollama":
			client, err := ollama.NewClient(ollamaURL, "")
			if err != nil {
				return SingleProviderModelsMsg{
					ProviderID: providerID,
					Models:     nil,
					Err:        err,
				}
			}

			ctx := context.Background()
			modelInfos, err := client.ListModels(ctx)
			if err != nil {
				return SingleProviderModelsMsg{
					ProviderID: providerID,
					Models:     nil,
					Err:        err,
				}
			}

			for i := range modelInfos {
				modelInfos[i].Provider = "ollama"
				modelInfos[i].InternalName = modelInfos[i].Name
			}

			models = modelInfos

		default:
			providerType := MapProviderIDToType(providerID)
			p, err := NewProvider(Config{
				Type:    providerType,
				BaseURL: baseURL,
				APIKey:  apiKey,
				Model:   "",
			})

			if err != nil {
				return SingleProviderModelsMsg{
					ProviderID: providerID,
					Models:     nil,
					Err:        err,
				}
			}

			ctx := context.Background()
			fetchedModels, err := p.ListModels(ctx)
			if err != nil {
				return SingleProviderModelsMsg{
					ProviderID: providerID,
					Models:     nil,
					Err:        err,
				}
			}

			models = fetchedModels
		}

		if config.Debug {
			config.DebugLog.Printf("[Provider] Fetched %d models from provider %s", len(models), providerID)
		}

		return SingleProviderModelsMsg{
			ProviderID: providerID,
			Models:     models,
			Err:        nil,
		}
	}
}
