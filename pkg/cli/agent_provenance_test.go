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
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// THE STAMP MUST COVER EVERYTHING IT COMPILES, NOT EVERYTHING THAT ENDS
// IN .go.
//
// The stage was all-Go until it started shipping a vendored dependency.
// protobuf go:embeds internal/editiondefaults/editions_defaults.binpb,
// which is COMPILED INTO the agent, and the hash pattern was "*.go" -
// REPRODUCED: mutating that file left the provenance hash byte-identical,
// so the stamp certified less than it compiled.
//
// TestStagedADKIsCoveredByTheProvenanceHash could not catch it: it alters
// a .go file, which the old pattern did see. This test alters the one
// kind of input the old pattern was blind to, which is why it is separate
// rather than another assertion there.
func TestEmbeddedNonGoAssetsAreCoveredByTheProvenanceHash(t *testing.T) {
	product.Set("gapi")

	real := testADKSource(t)

	src := t.TempDir()
	srcPath := filepath.Join(src, "embedded.go.service")
	if err := os.WriteFile(srcPath, []byte(standaloneAgentSource), 0600); err != nil {
		t.Fatalf("write agent source: %v", err)
	}

	t.Setenv(product.EnvKey("GO_ADK"), real.Dir)
	_, first, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("first build: %v", err)
	}

	// SAME TREE, SECOND BUILD. Determinism is asserted alongside
	// sensitivity because one without the other is what let the gap sit:
	// a hash that simply changed every time would also have passed a
	// first != second assertion (GAPI-DIV-103).
	_, again, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("repeat build: %v", err)
	}
	if first != again {
		t.Fatalf("the same tree hashed differently: %s then %s", first, again)
	}

	// A VERBATIM copy, then a change to ONE non-.go file.
	//
	// copyADKWithMarker also edits run.go, and a .go edit is exactly what
	// the old pattern DID see - a test built on it would have gone green
	// against the defect. The marker is undone first so the embedded asset
	// is the only difference between the two builds.
	altered := copyADKWithMarker(t, real)
	runGo := filepath.Join(altered.Dir, adkRelDir, "run.go")
	original, err := os.ReadFile(filepath.Join(real.Dir, adkRelDir, "run.go"))
	if err != nil {
		t.Fatalf("read original run.go: %v", err)
	}
	if err := os.WriteFile(runGo, original, 0600); err != nil {
		t.Fatalf("undo the .go marker: %v", err)
	}

	embedded := filepath.Join(altered.Dir, protobufRelDir,
		"internal", "editiondefaults", "editions_defaults.binpb")
	data, err := os.ReadFile(embedded)
	if err != nil {
		t.Fatalf("embedded asset not present in the staged ADK: %v", err)
	}
	if err := os.WriteFile(embedded, append(data, 0x00), 0600); err != nil {
		t.Fatalf("perturb embedded asset: %v", err)
	}

	t.Setenv(product.EnvKey("GO_ADK"), altered.Dir)
	_, second, err := buildGoAgent(srcPath, t.TempDir())
	if err != nil {
		t.Fatalf("build against altered asset: %v", err)
	}

	if first == second {
		t.Fatalf("changing a compiled-in non-.go asset left the provenance hash at %s", first)
	}
}
