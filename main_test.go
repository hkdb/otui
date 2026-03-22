package main

import (
	"errors"
	"strings"
	"testing"
)

func TestBuildCredentialErrorMessage(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantChecks []string
	}{
		{
			"decrypt_error",
			errors.New("failed to decrypt credentials"),
			[]string{
				"Failed to load credentials",
				"corrupted or encrypted",
				"different SSH key",
				"credentials.enc",
			},
		},
		{
			"generic_error",
			errors.New("something went wrong"),
			[]string{
				"Failed to load credentials",
				"something went wrong",
				"check your configuration",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := buildCredentialErrorMessage(tt.err)
			for _, check := range tt.wantChecks {
				if !strings.Contains(msg, check) {
					t.Errorf("message missing %q\ngot: %s", check, msg)
				}
			}
		})
	}
}
