// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// protoAgentSource is an agent whose ADK-facing work needs the generated
// gapi types. It does not import them itself - an agent imports nothing
// by default - but building it must link them, because operator decision
// 37 puts the ADK's control channel on protobuf over an inherited fd.
const protoAgentSource = `package agent

import "context"

const (
	ID          = "protoprobe"
	Type        = "service"
	Version     = "1.0.0"
	Description = "Built against an ADK that speaks protobuf"
)

func Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
`

// TestStagedAgentLinksTheSharedProtoPackage is the packaging half of the
// GAPI-DIV-087/-099 control channel.
//
// Operator decision 38: the fd carries protobuf, because the manifesto's
// FFI exception is named specifically so that a boundary which IS
// inter-process does not get a second one. Decision on duplicate
// registration: SHARE ONE PACKAGE - the kernel and the ADK import the
// same generated gapi types, so nothing can register `gapi/v1/*.proto`
// into the global protoregistry twice.
//
// Sharing one package is why the shipped tree is the kernel's own module
// rather than a synthesized `adk/go` module: a renamed module cannot
// import github.com/goppydae/gapi/pkg/proto, so the checkout tier and
// the install tier would resolve DIFFERENT packages, which is the
// duplicate-registration panic by another route.
//
// The assertion is that the stage BUILDS, offline. That is the only
// assertion worth making here: the staged module either resolves both
// the shared gapi module and the protobuf runtime with GOPROXY=off, or
// it does not.
func TestStagedAgentLinksTheSharedProtoPackage(t *testing.T) {
	product.Set("gapi")

	adk := testADKSource(t)
	t.Setenv(product.EnvKey("GO_ADK"), adk.Dir)

	src := filepath.Join(t.TempDir(), "protoprobe.go.service")
	if err := os.WriteFile(src, []byte(protoAgentSource), 0600); err != nil {
		t.Fatalf("write agent source: %v", err)
	}

	bin, _, err := buildGoAgent(src, t.TempDir())
	if err != nil {
		t.Fatalf("build against a protobuf-speaking ADK: %v", err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("agent binary absent: %v", err)
	}
}

// TestShippedADKCarriesTheProtobufRuntime asserts the resolver reports a
// tree that cannot supply the runtime, rather than letting it surface as
// a compile error in generated code.
//
// This is loadGoADK's existing standard applied to the new requirement:
// it already validates a FILE rather than the directory's existence,
// because an empty share/<product>/go is what a half-finished install
// leaves behind and it would otherwise be accepted and fail later.
func TestShippedADKCarriesTheProtobufRuntime(t *testing.T) {
	product.Set("gapi")

	incomplete := t.TempDir()
	if err := os.MkdirAll(filepath.Join(incomplete, "adk", "go", "agent"), 0750); err != nil {
		t.Fatalf("stage incomplete ADK: %v", err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, "adk", "go", "agent", "run.go"),
		[]byte("package agent\n"), 0600); err != nil {
		t.Fatalf("write run.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, "go.mod"),
		[]byte("module github.com/goppydae/gapi\n\ngo 1.26.0\n"), 0600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	_, err := loadGoADK(incomplete, "incomplete install")
	if err == nil {
		t.Fatal("an ADK tree with no protobuf runtime was accepted")
	}
	if !strings.Contains(err.Error(), "protobuf") {
		t.Errorf("error does not name the missing runtime: %v", err)
	}
}
