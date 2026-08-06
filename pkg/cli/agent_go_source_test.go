// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protodelim"

	"github.com/goppydae/gapi/core/product"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
)

// TestGeneratedMain_NamesTheEmbeddingProduct pins the kernel's
// disappearing act (GAPI-DIV-061): the generated header names the command
// that built the agent, and under goblind that command is goblinctl. A
// literal "gapictl" here would put a name goblin's operators have never
// heard of into the top of every agent they build.
func TestGeneratedMain_NamesTheEmbeddingProduct(t *testing.T) {
	src := writeAgent(t, t.TempDir(), "probe.go.service", noCtxAgent)
	d, err := scanGoAgent(src)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	product.Set("goblin")
	t.Cleanup(func() { product.Set("gapi") })

	got := string(d.generateMain(adkImportPath))
	if !strings.Contains(got, "goblinctl agent build") {
		t.Errorf("generated header does not name the embedding product:\n%s",
			strings.SplitN(got, "\n", 2)[0])
	}
	if strings.Contains(got, "gapictl") {
		t.Errorf("generated header hardcodes the kernel's command name:\n%s",
			strings.SplitN(got, "\n", 2)[0])
	}
}

// serviceAgent is the target form from design/gapi-architecture.md: a
// single file, no main, no flag parsing, no describe JSON, no signals.
const serviceAgent = `package agent

import "context"

const (
	ID          = "probe_service"
	Type        = "service"
	Version     = "2.1.0"
	Description = "A probe"
	Enabled     = true
)

var (
	Requires    = []string{"database"}
	WantedBy    = []string{"local.target"}
	CPULimit    = 0.5
	MemoryLimit = "512MB"
)

func Initialize() error { return nil }

func Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func Stop() error { return nil }
`

// noCtxAgent exercises the second accepted Start signature and the
// Python declaration spellings.
const noCtxAgent = `package agent

const (
	ID   = "probe_timer"
	TYPE = "timer"
)

var SCHEDULE = "OnUnitActiveSec=60s"

func Start() error { return nil }
`

func writeAgent(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatalf("write agent: %v", err)
	}
	return p
}

// buildAssembled assembles and compiles the agent, returning the binary.
//
// The staging directory is now a plain t.TempDir(): assembleGoAgent
// brings the ADK with it and writes the module files, so the stage
// resolves everything locally and does not have to live inside the kernel
// module (GAPI-DIV-092). The build asserts that by running with
// GOPROXY=off - if the stage ever stops being self-contained, this fails
// rather than quietly reaching the proxy on a connected machine.
func buildAssembled(t *testing.T, srcPath string) string {
	t.Helper()

	// The generated main names the building command, which derives from
	// the product identity. A test binary declares none, and reading an
	// undeclared identity panics by design (GAPI-DIV-061).
	product.Set("gapi")

	adk := testADKSource(t)
	stage := t.TempDir()

	if err := assembleGoAgent(srcPath, stage, adk); err != nil {
		t.Fatalf("assemble: %v", err)
	}

	bin := filepath.Join(t.TempDir(), "agent")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = stage
	build.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := build.CombinedOutput(); err != nil {
		gen, _ := os.ReadFile(filepath.Join(stage, "main.go"))
		t.Fatalf("the assembled package does not compile: %v\n%s\n--- generated main ---\n%s",
			err, out, gen)
	}
	return bin
}

// testADKSource resolves the ADK out of the checkout these tests run in.
// pkg/cli is two directories below the repository root, and the root IS
// the shared module the ADK ships as (decision 38).
func testADKSource(t *testing.T) goADK {
	t.Helper()
	adk, err := loadGoADK(filepath.Join("..", ".."), "test checkout")
	if err != nil {
		t.Fatalf("locate ADK source: %v", err)
	}
	return adk
}

// TestAssembledAgent_BuildsAndDescribes is the end-to-end proof that a
// file in the target form becomes a working agent binary. It compiles the
// generated package, which is the assertion that matters: a generator
// checked only against expected output can emit source that does not
// build.
func TestAssembledAgent_BuildsAndDescribes(t *testing.T) {
	src := writeAgent(t, t.TempDir(), "probe_service.go.service", serviceAgent)
	bin := buildAssembled(t, src)

	out, err := exec.Command(bin, "--describe").Output()
	if err != nil {
		t.Fatalf("--describe: %v", err)
	}

	var env struct {
		Describe struct {
			ID           string   `json:"id"`
			Type         string   `json:"type"`
			Version      string   `json:"version"`
			Language     string   `json:"language"`
			Enabled      bool     `json:"enabled"`
			Capabilities []string `json:"capabilities"`
			Requires     []string `json:"requires"`
			WantedBy     []string `json:"wanted_by"`
			CPULimit     string   `json:"cpu_limit"`
			MemoryLimit  string   `json:"memory_limit"`
		} `json:"describe"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("describe is not JSON: %v\n%s", err, out)
	}
	d := env.Describe

	for _, c := range []struct{ got, want, what string }{
		{d.ID, "probe_service", "id"},
		{d.Type, "service", "type"},
		{d.Version, "2.1.0", "version"},
		{d.Language, "go", "language"},
		{strings.Join(d.Capabilities, ","), "initialize,start,stop", "capabilities"},
		{strings.Join(d.Requires, ","), "database", "requires"},
		{strings.Join(d.WantedBy, ","), "local.target", "wanted_by"},
		{d.MemoryLimit, "512MB", "memory_limit"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.what, c.got, c.want)
		}
	}
	// A float CPULimit must survive onto a string wire field. This is the
	// case a literal-transcribing generator gets wrong.
	if d.CPULimit != "0.5" {
		t.Errorf("cpu_limit = %q, want %q", d.CPULimit, "0.5")
	}
	if !d.Enabled {
		t.Error("an agent declaring Enabled = true described as disabled")
	}
}

// TestAssembledAgent_AcceptsTheSupervisorsStartVerb is the regression for
// GAPI-DIV-052 at the level that actually failed: a BUILT BINARY invoked
// the way GoAgent.Start invokes it. The package-level test in adk/go/agent
// covers dispatch; this one covers the whole chain, which is where the
// defect lived - every template hand-wrote a flag set declaring only
// -describe, so flag.Parse rejected --start and the process exited 2.
func TestAssembledAgent_AcceptsTheSupervisorsStartVerb(t *testing.T) {
	src := writeAgent(t, t.TempDir(), "probe_service.go.service", serviceAgent)
	bin := buildAssembled(t, src)

	// The control channel is an inherited descriptor, so this test now
	// passes one exactly as the supervisor does (operator decision 37).
	// It used to read control events off the child's STDOUT, which is
	// what GAPI-DIV-099 removed - stdout carries logs and only logs.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("control pipe: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	cmd := exec.Command(bin, "--start")
	cmd.ExtraFiles = []*os.File{w}
	cmd.Env = append(os.Environ(), "ADK_CONTROL_FD=3")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// The parent's copy must close, or the reader never sees EOF.
	_ = w.Close()
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// Waiting for RUNNING is what distinguishes a process that started
	// from one that exited 2 in milliseconds.
	got := make(chan []string, 1)
	go func() { got <- readStatesUntil(r, "RUNNING") }()

	select {
	case states := <-got:
		if len(states) == 0 || states[0] != "PENDING" {
			t.Errorf("states %v, want them to open with PENDING", states)
		}
		if states[len(states)-1] != "RUNNING" {
			t.Errorf("states %v, want them to reach RUNNING", states)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the agent never reported running")
	}
}

// readStatesUntil collects lifecycle states from a control channel until
// want is seen or the stream ends.
func readStatesUntil(r io.Reader, want string) []string {
	var out []string
	br := bufio.NewReader(r)
	for {
		var frame gapiv1.AgentControl
		if err := protodelim.UnmarshalFrom(br, &frame); err != nil {
			return out
		}
		st := frame.GetStatus()
		if st == nil {
			continue
		}
		out = append(out, st.GetState())
		if st.GetState() == want {
			return out
		}
	}
}

func TestAssembledAgent_AdaptsAStartWithoutContext(t *testing.T) {
	src := writeAgent(t, t.TempDir(), "probe_timer.go.timer", noCtxAgent)
	bin := buildAssembled(t, src)

	out, err := exec.Command(bin, "--describe").Output()
	if err != nil {
		t.Fatalf("--describe: %v", err)
	}

	var env struct {
		Describe struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Schedule string `json:"schedule"`
		} `json:"describe"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("describe is not JSON: %v\n%s", err, out)
	}

	// TYPE and SCHEDULE are the Python spellings; an author porting an
	// agent between languages must not have to rename declarations.
	if env.Describe.Type != "timer" {
		t.Errorf("type = %q, want %q - the Python spelling was not accepted", env.Describe.Type, "timer")
	}
	if env.Describe.Schedule != "OnUnitActiveSec=60s" {
		t.Errorf("schedule = %q, want %q", env.Describe.Schedule, "OnUnitActiveSec=60s")
	}
}

func TestScanGoAgent_RejectsAFileWithoutStart(t *testing.T) {
	src := writeAgent(t, t.TempDir(), "broken.go.service", "package agent\n\nconst ID = \"broken\"\n")

	_, err := scanGoAgent(src)
	if err == nil {
		t.Fatal("a file declaring no Start was accepted")
	}
	if !strings.Contains(err.Error(), "Start") {
		t.Errorf("error %q does not name the missing Start", err)
	}
}

// TestScanGoAgent_IgnoresDeclarationsInsideFunctions guards the scan's
// scope. A local variable named Version must not become the agent's
// version - the walk visits top-level declarations only.
func TestScanGoAgent_IgnoresDeclarationsInsideFunctions(t *testing.T) {
	body := `package agent

const ID = "scoped"

func Start() error {
	const Version = "9.9.9"
	_ = Version
	return nil
}
`
	src := writeAgent(t, t.TempDir(), "scoped.go.service", body)
	d, err := scanGoAgent(src)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if _, ok := d.declared["Version"]; ok {
		t.Error("a Version declared inside a function was scanned as metadata")
	}
}
