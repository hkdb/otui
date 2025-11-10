package provider

import (
	"otui/model"
	"testing"
)

// TestOllamaProviderImplementsInterface is a compile-time check that OllamaProvider
// implements the Provider interface. This test will fail to compile if the interface
// is not satisfied.
func TestOllamaProviderImplementsInterface(t *testing.T) {
	var _ model.Provider = (*OllamaProvider)(nil)
}

// Note: Integration tests that require a running Ollama server are deferred to Phase 8.
// The interface contract tests in interface_test.go will eventually test OllamaProvider
// once we can mock or provide test fixtures for the ollama.Client dependency.
