package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// GetConfigDir returns the platform-specific configuration directory
// Linux/Mac: ~/.config/otui
// Windows: C:\Users\username\.config\otui
func GetConfigDir() string {
	if runtime.GOOS == "windows" {
		userProfile := os.Getenv("USERPROFILE")
		return filepath.Join(userProfile, ".config", "otui")
	}

	home := os.Getenv("HOME")
	return filepath.Join(home, ".config", "otui")
}

// GetDefaultDataDir returns the platform-specific default data directory
// Linux/Mac: ~/.local/share/otui
// Windows: C:\Users\username\AppData\Local\otui
func GetDefaultDataDir() string {
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			userProfile := os.Getenv("USERPROFILE")
			localAppData = filepath.Join(userProfile, "AppData", "Local")
		}
		return filepath.Join(localAppData, "otui")
	}

	home := os.Getenv("HOME")
	return filepath.Join(home, ".local", "share", "otui")
}

// GetCacheDir returns the platform-specific cache directory for OTUI
// This is where temporary files should live (never synced to cloud)
// Linux/Mac: ~/.cache/otui
// Windows: C:\Users\username\AppData\Local\otui
func GetCacheDir() string {
	if runtime.GOOS == "windows" {
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			userProfile := os.Getenv("USERPROFILE")
			localAppData = filepath.Join(userProfile, "AppData", "Local")
		}
		return filepath.Join(localAppData, "otui")
	}

	home := os.Getenv("HOME")
	return filepath.Join(home, ".cache", "otui")
}

// GetSettingsFilePath returns the path to settings.toml
func GetSettingsFilePath() string {
	return filepath.Join(GetConfigDir(), "settings.toml")
}

// GetHomeDir returns the user's home directory across platforms
// Windows: %USERPROFILE% (C:\Users\username)
// Linux/Mac: $HOME (/home/username)
func GetHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("USERPROFILE")
		if home == "" {
			// Fallback: HOMEDRIVE + HOMEPATH
			home = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		}
		if home == "" {
			// Last resort fallback
			home = "C:\\"
		}
		return home
	}
	home := os.Getenv("HOME")
	if home == "" {
		home = "/"
	}
	return home
}

// ExpandPath expands ~ and environment variables in a path
func ExpandPath(path string) string {
	if path == "" {
		return path
	}

	// Expand ~
	if strings.HasPrefix(path, "~/") {
		home := GetHomeDir()
		path = filepath.Join(home, path[2:])
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	// Clean the path
	return filepath.Clean(path)
}

// EnsureDir creates a directory if it doesn't exist (0700 - user-only access)
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0700)
}

// IsEmptyDir checks if a directory is empty
func IsEmptyDir(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// FileExists checks if a file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// NormalizeDataDirectory normalizes a data directory path by ensuring it ends with /otui
// or uses an existing otui/ subfolder if present
func NormalizeDataDirectory(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("data directory path cannot be empty")
	}

	expanded := ExpandPath(input)

	// Case 1: Path ends with /otui - use it directly
	if filepath.Base(expanded) == "otui" {
		return expanded, nil
	}

	// Case 2: Check if otui/ subfolder exists
	otuiPath := filepath.Join(expanded, "otui")
	if _, err := os.Stat(otuiPath); err == nil {
		// otui/ subfolder exists - use it
		return otuiPath, nil
	}

	// Case 3: No otui/ subfolder - will be created later
	// Return path with otui/ appended
	return otuiPath, nil
}

// GetTempDir returns the path to the secure temp directory
// Always uses cache directory, never data directory (to avoid cloud sync)
func GetTempDir() string {
	return filepath.Join(GetCacheDir(), "tmp")
}

// GetEditorTempFile returns the path to the reusable editor temp file
func GetEditorTempFile() string {
	return filepath.Join(GetTempDir(), "editor.txt")
}

// EnsureDataDirPermissions ensures data directory has 0700 permissions
func EnsureDataDirPermissions(dataDir string) error {
	info, err := os.Stat(dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dataDir, 0700)
		}
		return err
	}

	// Check permissions (mask with 0777 to get permission bits)
	currentPerms := info.Mode().Perm()
	if currentPerms != 0700 {
		return os.Chmod(dataDir, 0700)
	}
	return nil
}

// CleanupTempDir removes the temp directory if it exists
func CleanupTempDir() error {
	tmpDir := GetTempDir()
	if _, err := os.Stat(tmpDir); err == nil {
		return os.RemoveAll(tmpDir)
	}
	return nil
}

// CreateTempDir creates the secure temp directory with 0700 permissions
func CreateTempDir() error {
	tmpDir := GetTempDir()
	return os.MkdirAll(tmpDir, 0700)
}

// ClearEditorTempFile clears the contents of the editor temp file
func ClearEditorTempFile() error {
	editorFile := GetEditorTempFile()
	if _, err := os.Stat(editorFile); err == nil {
		return os.WriteFile(editorFile, []byte(""), 0600)
	}
	return nil
}
