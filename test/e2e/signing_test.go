// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package e2e

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/crypto"
)

func TestAgentSigningEnforcement(t *testing.T) {
	// 1. Setup Environment
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Build gapid and gapictl
	gapidBin := filepath.Join(tmpDir, "gapid")
	gapictlBin := filepath.Join(tmpDir, "gapictl")

	// Build gapid and gapictl (paths relative to test/e2e)
	buildCmd := exec.Command("go", "build", "-o", gapidBin, "../../cmd/gapid")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build gapid: %v\n%s", err, out)
	}
	buildCmd2 := exec.Command("go", "build", "-o", gapictlBin, "../../cmd/gapictl")
	if out, err := buildCmd2.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build gapictl: %v\n%s", err, out)
	}

	// Generate Keys
	privKeyPath := filepath.Join(tmpDir, "key.pem")
	pubKeyPath := filepath.Join(tmpDir, "key.pub.hex")
	kp, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := kp.SavePrivate(privKeyPath); err != nil {
		t.Fatal(err)
	}
	if err := kp.SavePublic(pubKeyPath); err != nil {
		t.Fatal(err)
	}

	// Create a test agent
	agentName := "signed_agent.py.service"
	agentPath := filepath.Join(agentsDir, agentName)
	agentCode := `
import time
ID="signed_agent"
TYPE="service"
def start(stop_evt=None):
    while not stop_evt.is_set():
        time.sleep(0.1)
`
	if err := os.WriteFile(agentPath, []byte(agentCode), 0644); err != nil {
		t.Fatal(err)
	}

	// Helper to run gapid
	runGapid := func(ctx context.Context) (*exec.Cmd, string, error) {
		cmd := exec.CommandContext(ctx, gapidBin, "start")
		cmd.Env = append(os.Environ(),
			"GAPI_DEV_AGENTS="+agentsDir,
			"GAPI_VERIFY_KEY="+pubKeyPath,
			// GAPI_SKIP_ROOT_CHECK was set here and read by no code in
			// the tree - a no-op that read as a precondition. Dropped
			// while renaming the namespace (GAPI-DIV-059); the scan gate
			// added there now fails on a set-but-unread project name.
			"GAPI_TRANSPORT_TYPE=quic",           // Ensure consistent transport
			"GAPI_TRANSPORT_ADDRESS=127.0.0.1:0", // Random port
			"GAPI_PY_ADK=../../adk/python",
		)
		// Capture stdout/stderr
		logFile := filepath.Join(tmpDir, "gapid.log")
		f, err := os.Create(logFile)
		if err != nil {
			return nil, "", err
		}
		cmd.Stdout = f
		cmd.Stderr = f

		if err := cmd.Start(); err != nil {
			if cerr := f.Close(); cerr != nil {
				return nil, "", fmt.Errorf("%w (also failed to close log file: %w)", err, cerr)
			}
			return nil, "", err
		}
		return cmd, logFile, nil
	}

	// Scenario 1: Unsigned Agent
	// Start gapid
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd, logFile, err := runGapid(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Give it time to discover
	time.Sleep(3 * time.Second)

	// Check logs for rejection
	logs, _ := os.ReadFile(logFile)
	if !strings.Contains(string(logs), "integrity check failed") && !strings.Contains(string(logs), "skipping startup") {
		t.Errorf("Expected rejection of unsigned agent, but logs didn't show it. Logs:\n%s", string(logs))
	}

	// Stop gapid
	cancel()
	if err := cmd.Wait(); err != nil {
		// gapid was cancelled/killed above; log the exit for visibility.
		t.Logf("gapid exit: %v", err)
	}

	// Scenario 2: Signed Agent
	// Sign the agent
	sig := kp.Sign([]byte(agentCode))
	sigPath := agentPath + ".sig"
	if err := os.WriteFile(sigPath, []byte(hex.EncodeToString(sig)), 0644); err != nil {
		t.Fatal(err)
	}

	// Restart gapid
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	cmd2, logFile2, err := runGapid(ctx2)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	logs2, _ := os.ReadFile(logFile2)
	if !strings.Contains(string(logs2), "registered agent") {
		t.Errorf("Expected success for signed agent, but logs didn't show registration. Logs:\n%s", string(logs2))
	}

	// Stop gapid
	cancel2()
	if err := cmd2.Wait(); err != nil {
		// gapid was cancelled/killed above; log the exit for visibility.
		t.Logf("gapid exit: %v", err)
	}

	// Scenario 3: Tampered Agent
	// Modify agent content but keep old signature
	tamperedCode := agentCode + "\n# malicious"
	if err := os.WriteFile(agentPath, []byte(tamperedCode), 0644); err != nil {
		t.Fatal(err)
	}

	// Restart gapid
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel3()

	cmd3, logFile3, err := runGapid(ctx3)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)

	logs3, _ := os.ReadFile(logFile3)
	if !strings.Contains(string(logs3), "integrity check failed") {
		t.Errorf("Expected rejection of tampered agent, but logs didn't show it. Logs:\n%s", string(logs3))
	}

	cancel3()
	if err := cmd3.Wait(); err != nil {
		// gapid was cancelled/killed above; log the exit for visibility.
		t.Logf("gapid exit: %v", err)
	}
}
