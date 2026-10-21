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
