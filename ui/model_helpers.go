package ui

import (
	"strings"

	"otui/ollama"
)

// IsCurrentModel checks if a model matches the current model name.
// Handles the difference between display names and internal names across providers.
//
// For Ollama: Name == InternalName (e.g., "llama3.2:latest")
// For OpenRouter/others: Name is stripped, InternalName has full path
//   - Name: "llama-3.1-8b:free"
//   - InternalName: "meta-llama/llama-3.1-8b:free"
//
// This ensures "(current)" marker and model selection work across all providers.
func IsCurrentModel(model ollama.ModelInfo, currentModel string) bool {
	// Try InternalName first (correct for all providers)
	if model.InternalName == currentModel {
		return true
	}
	// Fallback to Name (for backwards compatibility or edge cases)
	if model.Name == currentModel {
		return true
	}
	return false
}

// FindModelByName finds a model in a list by matching against current model name.
// Returns the index and model info, or -1 and nil if not found.
// Uses the same matching logic as IsCurrentModel.
func FindModelByName(models []ollama.ModelInfo, modelName string) (int, *ollama.ModelInfo) {
	for i, model := range models {
		if IsCurrentModel(model, modelName) {
			return i, &model
		}
	}
	return -1, nil
}

// ModelSupportsTools checks if a model supports tool calling (MCP plugins).
// This is provider-aware and covers all providers, not just Ollama.
//
// Tool support by provider:
//   - Ollama: Curated list (llama3.2, qwen2.5, mistral, etc.)
//   - OpenRouter: Most models, with some exceptions
//   - Anthropic: ALL Claude models support tools
//   - OpenAI: Most models (gpt-4, gpt-3.5-turbo, etc.)
//
// This replaces the Ollama-only ollama.ModelSupportsToolCalling() check
// to provide consistent tool indicators across all providers.
func ModelSupportsTools(model ollama.ModelInfo) bool {
	switch model.Provider {
	case "ollama":
		// Use existing Ollama detection logic (checks against curated list)
		return ollama.ModelSupportsToolCalling(model.Name)

	case "anthropic":
		// ALL Anthropic/Claude models support tools natively
		return true

	case "openai":
		// Most OpenAI models support tools
		modelLower := strings.ToLower(model.InternalName)

		// Models that support tools
		toolSupportedPrefixes := []string{
			"gpt-4",         // gpt-4, gpt-4-turbo, gpt-4o, etc.
			"gpt-3.5-turbo", // gpt-3.5-turbo variants
		}

		for _, prefix := range toolSupportedPrefixes {
			if strings.HasPrefix(modelLower, prefix) {
				return true
			}
		}
		return false

	case "openrouter":
		// OpenRouter: Most models support tools
		// Exception: Some very small models may not
		modelLower := strings.ToLower(model.InternalName)

		// Models known NOT to support tools well
		noToolSupport := []string{
			"meta-llama/llama-3.2-1b", // Too small for reliable tool use
			"meta-llama/llama-3.2-3b", // Too small for reliable tool use
		}

		for _, prefix := range noToolSupport {
			if strings.Contains(modelLower, prefix) {
				return false
			}
		}

		// Default: assume OpenRouter models support tools
		// (Most do, and it's better to show indicator and let runtime fail
		//  than hide indicator and confuse users about plugin compatibility)
		return true

	default:
		// Unknown provider - conservative approach (don't show tool indicator)
		return false
	}
}
