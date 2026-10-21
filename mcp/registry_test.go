package mcp

import "testing"

func TestRegistryReloadConcurrentWithGetAll(t *testing.T) {
	r, err := NewRegistry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// A registry refresh (Cmd goroutine) while the plugin manager renders
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = r.Load()
	}()
	_ = r.GetAll()
	_ = r.Search("x")
	<-done
}
