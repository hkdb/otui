package provider

// ModelMetadata contains metadata about a model's capabilities
type ModelMetadata struct {
	ContextWindow int  `json:"context_window"`
	MaxOutput     int  `json:"max_output"`
	SupportsTools bool `json:"supports_tools"`
}

// Fallback metadata for when API doesn't provide context window info
// These are conservative estimates for common models

// OllamaFallbackMetadata contains fallback metadata for Ollama models
var OllamaFallbackMetadata = map[string]ModelMetadata{
	// Llama 3.x family
	"llama3.1":        {ContextWindow: 128000, MaxOutput: 8192, SupportsTools: true},
	"llama3.2":        {ContextWindow: 128000, MaxOutput: 8192, SupportsTools: true},
	"llama3":          {ContextWindow: 8192, MaxOutput: 4096, SupportsTools: true},

	// Qwen family
	"qwen2.5":         {ContextWindow: 32768, MaxOutput: 8192, SupportsTools: true},
	"qwen2.5-coder":   {ContextWindow: 32768, MaxOutput: 8192, SupportsTools: true},
	"qwen3":           {ContextWindow: 131072, MaxOutput: 8192, SupportsTools: true},
	"qwen3-coder":     {ContextWindow: 131072, MaxOutput: 8192, SupportsTools: true},

	// Mistral family
	"mistral":         {ContextWindow: 32768, MaxOutput: 8192, SupportsTools: true},
	"mistral-nemo":    {ContextWindow: 128000, MaxOutput: 8192, SupportsTools: true},
	"m2":              {ContextWindow: 128000, MaxOutput: 8192, SupportsTools: true},
	"mistral2":        {ContextWindow: 128000, MaxOutput: 8192, SupportsTools: true},

	// DeepSeek
	"deepseek-coder":  {ContextWindow: 16384, MaxOutput: 4096, SupportsTools: false},
	"deepseek-coder-v2": {ContextWindow: 32768, MaxOutput: 8192, SupportsTools: true},

	// CodeLlama
	"codellama":       {ContextWindow: 16384, MaxOutput: 4096, SupportsTools: false},
}

// AnthropicFallbackMetadata contains fallback metadata for Anthropic models
var AnthropicFallbackMetadata = map[string]ModelMetadata{
	"claude-sonnet-4":              {ContextWindow: 200000, MaxOutput: 8192, SupportsTools: true},
	"claude-sonnet-4-5":            {ContextWindow: 200000, MaxOutput: 8192, SupportsTools: true},
	"claude-opus-4":                {ContextWindow: 200000, MaxOutput: 8192, SupportsTools: true},
	"claude-opus-4-5":              {ContextWindow: 200000, MaxOutput: 8192, SupportsTools: true},
	"claude-3-5-sonnet-20241022":   {ContextWindow: 200000, MaxOutput: 8192, SupportsTools: true},
	"claude-3-5-sonnet-20240620":   {ContextWindow: 200000, MaxOutput: 8192, SupportsTools: true},
	"claude-3-opus-20240229":       {ContextWindow: 200000, MaxOutput: 4096, SupportsTools: true},
	"claude-3-sonnet-20240229":     {ContextWindow: 200000, MaxOutput: 4096, SupportsTools: true},
	"claude-3-haiku-20240307":      {ContextWindow: 200000, MaxOutput: 4096, SupportsTools: true},
}

// OpenAIFallbackMetadata contains fallback metadata for OpenAI models
var OpenAIFallbackMetadata = map[string]ModelMetadata{
	"gpt-4o":                  {ContextWindow: 128000, MaxOutput: 16384, SupportsTools: true},
	"gpt-4o-mini":             {ContextWindow: 128000, MaxOutput: 16384, SupportsTools: true},
	"gpt-4-turbo":             {ContextWindow: 128000, MaxOutput: 4096, SupportsTools: true},
	"gpt-4":                   {ContextWindow: 8192, MaxOutput: 4096, SupportsTools: true},
	"gpt-3.5-turbo":           {ContextWindow: 16385, MaxOutput: 4096, SupportsTools: true},
}

// GetFallbackMetadata returns fallback metadata for a model based on its name
// This uses pattern matching on the model name to find the best match
func GetFallbackMetadata(modelName string, fallbackMap map[string]ModelMetadata) ModelMetadata {
	// Try exact match first
	if meta, ok := fallbackMap[modelName]; ok {
		return meta
	}

	// Try prefix matching - sort by length (longest first) to prefer specific matches
	// e.g., "llama3.2:latest" should match "llama3.2" (128k) not "llama3" (8k)
	var prefixes []string
	for prefix := range fallbackMap {
		prefixes = append(prefixes, prefix)
	}

	// Sort by length descending (longest prefixes first for most specific match)
	for i := 0; i < len(prefixes); i++ {
		for j := i + 1; j < len(prefixes); j++ {
			if len(prefixes[j]) > len(prefixes[i]) {
				prefixes[i], prefixes[j] = prefixes[j], prefixes[i]
			}
		}
	}

	// Now try matching with sorted prefixes (longest first)
	for _, prefix := range prefixes {
		if len(modelName) >= len(prefix) && modelName[:len(prefix)] == prefix {
			return fallbackMap[prefix]
		}
	}

	// Ultimate fallback (conservative)
	return ModelMetadata{
		ContextWindow: 8192,
		MaxOutput:     2048,
		SupportsTools: false,
	}
}
