package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSanitizesSessionFile(t *testing.T) {
	dir := t.TempDir()
	ss, err := NewSessionStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A crafted (e.g. imported) session file
	data := `{"id": "evil", "name": "Work\u001b]52;c;aGk=\u0007\nnotes", "model": "m",
		"messages": [{"role": "assistant", "content": "hi\r\u001b[2Kthere",
		"rendered": "\u001b[1mbold\u001b[0m\u001b[2J\u001b]8;;http://x\u001b\\link"}]}`
	if err := os.WriteFile(filepath.Join(dir, "sessions", "evil.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := ss.Load("evil")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(s.Name, "\x1b\x07\n") || strings.ContainsAny(s.Messages[0].Content, "\x1b\r") {
		t.Errorf("name %q / content %q not cleaned", s.Name, s.Messages[0].Content)
	}
	if got := s.Messages[0].Rendered; got != "\x1b[1mbold\x1b[0mlink" {
		t.Errorf("rendered = %q, want colors kept and other sequences removed", got)
	}

	list, err := ss.List()
	if err != nil || len(list) != 1 || strings.ContainsAny(list[0].Name, "\x1b\x07\n") {
		t.Errorf("listed name not cleaned: %+v (err %v)", list, err)
	}
}
