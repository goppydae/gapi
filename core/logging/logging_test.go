// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/config"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"trace", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"bogus", slog.LevelInfo},
	}
	for _, tt := range tests {
		if got := ParseLevel(tt.in); got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

// buildToFile builds a logger with a file sink so the emitted bytes can be
// asserted without capturing stdout.
func buildToFile(t *testing.T, level, format string) (*slog.Logger, func() string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	cfg := &config.LoggingConfig{
		Level:  level,
		Format: format,
		File:   config.FileOutputConfig{Enabled: true, Path: path, MaxSize: 1},
	}
	logger, closer, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return logger, func() string {
		if err := closer.Close(); err != nil {
			t.Fatalf("close sink: %v", err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read log file: %v", err)
		}
		return string(b)
	}
}

func TestBuild_JSONFormat(t *testing.T) {
	logger, read := buildToFile(t, "info", "json")
	logger.LogAttrs(context.Background(), slog.LevelInfo, "hello", slog.String("k", "v"))

	out := read()
	if !strings.Contains(out, `"msg":"hello"`) || !strings.Contains(out, `"k":"v"`) {
		t.Errorf("JSON output missing fields: %q", out)
	}
}

func TestBuild_ConsoleFormat(t *testing.T) {
	logger, read := buildToFile(t, "info", "console")
	logger.LogAttrs(context.Background(), slog.LevelInfo, "hello", slog.String("k", "v"))

	out := read()
	if !strings.Contains(out, "msg=hello") || !strings.Contains(out, "k=v") {
		t.Errorf("console output missing fields: %q", out)
	}
}

func TestBuild_LevelFilters(t *testing.T) {
	logger, read := buildToFile(t, "warn", "json")
	logger.LogAttrs(context.Background(), slog.LevelInfo, "filtered")
	logger.LogAttrs(context.Background(), slog.LevelWarn, "kept")

	out := read()
	if strings.Contains(out, "filtered") {
		t.Errorf("info record should be filtered at warn level: %q", out)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("warn record missing: %q", out)
	}
}

func TestBuild_LokiRejected(t *testing.T) {
	cfg := &config.LoggingConfig{
		Loki: config.LokiOutputConfig{Enabled: true},
	}
	if _, _, err := Build(cfg); err == nil {
		t.Fatal("Build with loki enabled should fail loudly (not implemented)")
	}
}
