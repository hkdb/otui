package config

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestDebugf(t *testing.T) {
	saved := DebugLog
	defer func() { DebugLog = saved }()

	DebugLog = nil
	Debugf("ignored %d", 1) // Must not panic when logging is off

	var buf bytes.Buffer
	DebugLog = log.New(&buf, "", log.Lshortfile)
	Debugf("hello %s", "world")

	got := buf.String()
	if !strings.HasPrefix(got, "debug_test.go:") || !strings.HasSuffix(got, "hello world\n") {
		t.Errorf("Debugf wrote %q, want the caller's file:line and the message", got)
	}
}
