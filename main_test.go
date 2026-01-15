package main

import (
	"testing"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		setValue     string
		want         string
	}{
		{
			name:         "returns environment variable when set",
			key:          "TEST_VAR_EXISTS",
			defaultValue: "default",
			setValue:     "actual",
			want:         "actual",
		},
		{
			name:         "returns default when environment variable not set",
			key:          "TEST_VAR_NOT_EXISTS",
			defaultValue: "default",
			setValue:     "",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setValue != "" {
				t.Setenv(tt.key, tt.setValue)
			}
			if got := getEnv(tt.key, tt.defaultValue); got != tt.want {
				t.Errorf("getEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateInputs(t *testing.T) {
	tests := []struct {
		name        string
		server      string
		token       string
		projectID   int
		startJob    int
		endJob      int
		concurrency int
		wantErr     bool
	}{
		{
			name:        "valid inputs",
			server:      "gitlab.example.com",
			token:       "valid-token",
			projectID:   1,
			startJob:    1,
			endJob:      10,
			concurrency: 5,
			wantErr:     false,
		},
		{
			name:        "empty server",
			server:      "",
			token:       "valid-token",
			projectID:   1,
			startJob:    1,
			endJob:      10,
			concurrency: 5,
			wantErr:     true,
		},
		{
			name:        "empty token",
			server:      "gitlab.example.com",
			token:       "",
			projectID:   1,
			startJob:    1,
			endJob:      10,
			concurrency: 5,
			wantErr:     true,
		},
		{
			name:        "invalid project ID",
			server:      "gitlab.example.com",
			token:       "valid-token",
			projectID:   0,
			startJob:    1,
			endJob:      10,
			concurrency: 5,
			wantErr:     true,
		},
		{
			name:        "invalid start job",
			server:      "gitlab.example.com",
			token:       "valid-token",
			projectID:   1,
			startJob:    0,
			endJob:      10,
			concurrency: 5,
			wantErr:     true,
		},
		{
			name:        "end job less than start job",
			server:      "gitlab.example.com",
			token:       "valid-token",
			projectID:   1,
			startJob:    10,
			endJob:      5,
			concurrency: 5,
			wantErr:     true,
		},
		{
			name:        "concurrency too low",
			server:      "gitlab.example.com",
			token:       "valid-token",
			projectID:   1,
			startJob:    1,
			endJob:      10,
			concurrency: 0,
			wantErr:     true,
		},
		{
			name:        "concurrency too high",
			server:      "gitlab.example.com",
			token:       "valid-token",
			projectID:   1,
			startJob:    1,
			endJob:      10,
			concurrency: 1001,
			wantErr:     true,
		},
		{
			name:        "job range too large",
			server:      "gitlab.example.com",
			token:       "valid-token",
			projectID:   1,
			startJob:    1,
			endJob:      1000001,
			concurrency: 5,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputs(tt.server, tt.token, tt.projectID, tt.startJob, tt.endJob, tt.concurrency)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateInputs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
