package schema

import (
	"testing"
)

func TestValidateID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid simple", "myagent", false},
		{"valid with underscore", "my_agent", false},
		{"valid with hyphen", "my-agent", false},
		{"valid alphanumeric", "agent123", false},
		{"empty", "", true},
		{"with spaces", "my agent", true},
		{"with special chars", "my@agent", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCPULimit(t *testing.T) {
	tests := []struct {
		name    string
		limit   string
		wantErr bool
	}{
		{"valid decimal", "0.5", false},
		{"valid integer", "1", false},
		{"valid millicpu", "500m", false},
		{"invalid negative", "-0.5", true},
		{"invalid zero", "0", true},
		{"invalid format", "abc", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCPULimit(tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCPULimit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateMemoryLimit(t *testing.T) {
	tests := []struct {
		name    string
		limit   string
		wantErr bool
	}{
		{"valid MB", "100MB", false},
		{"valid GB", "1GB", false},
		{"valid M", "512M", false},
		{"valid lowercase", "100mb", false},
		{"invalid no unit", "100", true},
		{"invalid negative", "-100MB", true},
		{"invalid format", "abc", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMemoryLimit(tt.limit)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMemoryLimit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		wantErr  bool
	}{
		{"valid OnUnitActiveSec", "OnUnitActiveSec=5s", false},
		{"valid OnBootSec", "OnBootSec=30s", false},
		{"valid OnStartupSec", "OnStartupSec=1m", false},
		{"valid raw duration", "5s", false},
		{"invalid duration", "OnUnitActiveSec=abc", true},
		{"invalid format", "InvalidPrefix=5s", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSchedule(tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSchedule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateListenStream(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{"valid with host", "0.0.0.0:8080", false},
		{"valid localhost", "127.0.0.1:9000", false},
		{"valid port only", ":8080", false},
		{"invalid no colon", "8080", true},
		{"invalid port", "0.0.0.0:99999", true},
		{"invalid format", "host:port:extra", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateListenStream(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateListenStream() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
