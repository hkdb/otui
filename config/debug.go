package config

import "fmt"

// Debugf writes a line to the debug log when OTUI_DEBUG is set and does
// nothing otherwise. The log line reports the caller's file and line.
func Debugf(format string, args ...any) {
	if DebugLog == nil {
		return
	}
	_ = DebugLog.Output(2, fmt.Sprintf(format, args...))
}

// DebugEnabled reports whether debug logging is on, for logging that needs
// extra work (like loops) beyond a single Debugf call
func DebugEnabled() bool {
	return DebugLog != nil
}
