//go:build e2e

package agents

import (
	"strings"
	"testing"
)

func TestValidateKiroWhoamiJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid account object",
			input:   `{"account":{"id":"123","provider":"builder-id"}}`,
			wantErr: false,
		},
		{
			name:        "null account",
			input:       `{"account":null}`,
			wantErr:     true,
			errContains: "account is null",
		},
		{
			name:        "missing account field",
			input:       `{"user":"someone"}`,
			wantErr:     true,
			errContains: "account is missing",
		},
		{
			name:        "invalid json",
			input:       `{`,
			wantErr:     true,
			errContains: "invalid whoami JSON",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateKiroWhoamiJSON([]byte(tt.input))
			if tt.wantErr && err == nil {
				t.Fatal("validateKiroWhoamiJSON() error = nil, want non-nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateKiroWhoamiJSON() error = %v, want nil", err)
			}
			if tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
				t.Fatalf("validateKiroWhoamiJSON() error = %v, want contains %q", err, tt.errContains)
			}
		})
	}
}

func TestIsTruthyEnvValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty", input: "", want: false},
		{name: "zero", input: "0", want: false},
		{name: "false lowercase", input: "false", want: false},
		{name: "false mixed", input: "False", want: false},
		{name: "one", input: "1", want: true},
		{name: "true", input: "true", want: true},
		{name: "yes-like non-empty", input: "enabled", want: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isTruthyEnvValue(tt.input)
			if got != tt.want {
				t.Fatalf("isTruthyEnvValue(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateKiroSIGV4Inputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		region          string
		accessKeyID     string
		secretAccessKey string
		wantErr         bool
		errContains     string
	}{
		{
			name:            "valid inputs",
			region:          "us-east-1",
			accessKeyID:     "AKIA...",
			secretAccessKey: "secret",
			wantErr:         false,
		},
		{
			name:            "missing region",
			region:          "",
			accessKeyID:     "AKIA...",
			secretAccessKey: "secret",
			wantErr:         true,
			errContains:     "AWS_REGION",
		},
		{
			name:            "missing access key",
			region:          "us-east-1",
			accessKeyID:     "",
			secretAccessKey: "secret",
			wantErr:         true,
			errContains:     "AWS_ACCESS_KEY_ID",
		},
		{
			name:            "missing secret key",
			region:          "us-east-1",
			accessKeyID:     "AKIA...",
			secretAccessKey: "",
			wantErr:         true,
			errContains:     "AWS_SECRET_ACCESS_KEY",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateKiroSIGV4Inputs(tt.region, tt.accessKeyID, tt.secretAccessKey)
			if tt.wantErr && err == nil {
				t.Fatal("validateKiroSIGV4Inputs() error = nil, want non-nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateKiroSIGV4Inputs() error = %v, want nil", err)
			}
			if tt.errContains != "" && (err == nil || !strings.Contains(err.Error(), tt.errContains)) {
				t.Fatalf("validateKiroSIGV4Inputs() error = %v, want contains %q", err, tt.errContains)
			}
		})
	}
}
