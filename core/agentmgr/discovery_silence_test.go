// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/goppydae/gapi/core/config"
)

// discoveryLogSink captures records so an assertion can be made about
// what discovery REPORTED, not only about what it returned.
//
// It deliberately does not reuse captureRecords from adopted_linux_test.go:
// that file is Linux-only and core/agentmgr builds elsewhere, so a
// dependency on it would make this test vanish on any other platform -
// which is the class of defect this very entry is about.
type discoveryLogSink struct {
	mu   sync.Mutex
	recs []slog.Record
}

func (s *discoveryLogSink) Enabled(context.Context, slog.Level) bool { return true }

func (s *discoveryLogSink) Handle(_ context.Context, r slog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = append(s.recs, r.Clone())
	return nil
}

func (s *discoveryLogSink) WithAttrs([]slog.Attr) slog.Handler { return s }
func (s *discoveryLogSink) WithGroup(string) slog.Handler      { return s }

func (s *discoveryLogSink) records() []slog.Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]slog.Record(nil), s.recs...)
}

func captureDiscoveryLogs(t *testing.T) *discoveryLogSink {
	t.Helper()
	s := &discoveryLogSink{}
	prev := slog.Default()
	slog.SetDefault(slog.New(s))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return s
}

// mentions reports whether any record at or above minLevel carries every
// one of the given substrings, across its message and its attributes.
func mentions(recs []slog.Record, minLevel slog.Level, want ...string) bool {
	for _, r := range recs {
		if r.Level < minLevel {
			continue
		}
		var sb strings.Builder
		sb.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool {
			sb.WriteString(" ")
			sb.WriteString(a.Key)
			sb.WriteString("=")
			sb.WriteString(a.Value.String())
			return true
		})
		line := sb.String()
		ok := true
		for _, w := range want {
			if !strings.Contains(line, w) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// TestDiscovery_ReportsAPythonAgentThatFailedToDescribe is the
// regression for GAPI-DIV-079.
//
// A file named *.py.timer has ALREADY declared itself an agent. If it
// then fails to describe - broken syntax, missing runner, bad metadata -
// discovery returned nil with the diagnostic commented out, so the
// daemon reported "agent discovery complete count=0", which is
// indistinguishable from an empty directory. Finding GAPI-DIV-077 that
// way took booting a VM and reading the discovery source.
//
// Asserting on the returned count would pass today and prove nothing.
// The assertion has to be that the failure was REPORTED, with the path
// and a reason.
func TestDiscovery_ReportsAPythonAgentThatFailedToDescribe(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.py.timer")
	if err := os.WriteFile(broken, []byte("ID = \"broken\"\nthis is not valid python\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	sink := captureDiscoveryLogs(t)

	am := NewAgentManager(nil, nil, "../../adk/python/agent/runner.py", false, nil)
	agents, err := am.discoverFromSinglePath(dir, config.PathTypeSystem)
	if err != nil {
		t.Fatalf("discoverFromSinglePath: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("a syntactically broken agent was discovered as %d agents", len(agents))
	}

	if !mentions(sink.records(), slog.LevelWarn, "broken.py.timer") {
		t.Error("a file that declared itself an agent and then failed to describe was discarded with no warning; count=0 is all an operator would see")
	}
}

// TestDiscovery_ReportsAnExecutableThatIsNotAnAgent covers the binary
// branch, which is the weaker case: an executable on a search path has
// declared nothing, so it may legitimately be an unrelated program.
//
// The assertion is deliberately about the LEVEL-INDEPENDENT property -
// that the skip is reported at all, naming the path and a reason. Whether
// that belongs at DEBUG or WARN is a judgement call about noise, and
// pinning it here would make a defensible change to it look like a
// regression. What must not happen is the skip going back to silent.
func TestDiscovery_ReportsAnExecutableThatIsNotAnAgent(t *testing.T) {
	dir := t.TempDir()
	// Not a script: no shebang and no interpreter, so exec fails with a
	// format error on any host. A shebang would make this test depend on
	// an interpreter path, which is not portable across the devshell and
	// a NixOS host.
	notAnAgent := filepath.Join(dir, "notanagent")
	if err := os.WriteFile(notAnAgent, []byte("this is not a program\n"), 0o700); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	sink := captureDiscoveryLogs(t)

	am := NewAgentManager(nil, nil, "../../adk/python/agent/runner.py", false, nil)
	agents, err := am.discoverFromSinglePath(dir, config.PathTypeSystem)
	if err != nil {
		t.Fatalf("discoverFromSinglePath: %v", err)
	}
	if len(agents) != 0 {
		t.Fatalf("a non-agent executable was discovered as %d agents", len(agents))
	}

	if !mentions(sink.records(), slog.LevelDebug, "notanagent") {
		t.Error("an executable was skipped with no record at any level; a real agent binary failing to describe would vanish the same way")
	}
}
