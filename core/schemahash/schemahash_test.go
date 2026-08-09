// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package schemahash

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// Linked for its side effect: registering the gapi.v1 descriptors in
	// the global registry. Without it this test binary hashes an empty
	// selection and every case below passes against nothing.
	_ "github.com/goppydae/gapi/pkg/proto"
)

// TestContractIsStableAndNonEmpty is the floor: a hash that is empty, or
// that changes between two calls in one process, cannot be compared
// across processes at all.
func TestContractIsStableAndNonEmpty(t *testing.T) {
	got := Contract()
	if got == "" {
		t.Fatal("Contract() is empty - nothing to compare across processes")
	}
	if len(got) != 64 {
		t.Fatalf("Contract() = %q, want 64 hex chars of BLAKE3", got)
	}
	if again := Contract(); again != got {
		t.Fatalf("Contract() is not stable: %q then %q", got, again)
	}
}

// TestContractCoversTheGapiPackage guards the filter. A hash computed
// over an empty selection is a constant, and a constant compares equal
// everywhere - which is a mismatch detector that can never fire.
func TestContractCoversTheGapiPackage(t *testing.T) {
	if len(gapiFiles()) == 0 {
		t.Fatalf("no %s files are linked into this test binary; the hash would "+
			"be a constant", gapiPackage)
	}
	if empty := contractFrom(emptyRanger{}); empty == Contract() {
		t.Fatal("the hash of an empty descriptor set equals the real one")
	}
}

type emptyRanger struct{}

func (emptyRanger) RangeFiles(func(protoreflect.FileDescriptor) bool) {}

// TestContractIgnoresRegistryOrder is the determinism guard. RangeFiles
// gives no ordering promise, so two processes could enumerate the same
// files in different orders - and an order-sensitive hash would then
// report skew between two identical binaries, which is worse than no
// detector at all.
func TestContractIgnoresRegistryOrder(t *testing.T) {
	files := gapiFiles()
	if len(files) < 2 {
		t.Fatalf("need at least 2 %s files to permute, got %d", gapiPackage, len(files))
	}

	forward := contractFrom(sliceRanger(files))
	reversed := make([]protoreflect.FileDescriptor, len(files))
	for i, fd := range files {
		reversed[len(files)-1-i] = fd
	}
	if got := contractFrom(sliceRanger(reversed)); got != forward {
		t.Fatalf("hash depends on enumeration order: %q forward, %q reversed",
			forward, got)
	}
}

// TestContractChangesWithTheContract proves the hash discriminates. A
// detector that returns the same value for two different descriptor sets
// reports "no skew" forever.
func TestContractChangesWithTheContract(t *testing.T) {
	files := gapiFiles()
	if len(files) < 2 {
		t.Fatalf("need at least 2 %s files, got %d", gapiPackage, len(files))
	}

	full := contractFrom(sliceRanger(files))
	fewer := contractFrom(sliceRanger(files[:len(files)-1]))
	if full == fewer {
		t.Fatal("dropping a file did not change the hash")
	}
}

// gapiFiles collects the linked descriptors the hash is computed over.
func gapiFiles() []protoreflect.FileDescriptor {
	var files []protoreflect.FileDescriptor
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if string(fd.Package()) == gapiPackage {
			files = append(files, fd)
		}
		return true
	})
	return files
}

type sliceRanger []protoreflect.FileDescriptor

func (s sliceRanger) RangeFiles(fn func(protoreflect.FileDescriptor) bool) {
	for _, fd := range s {
		if !fn(fd) {
			return
		}
	}
}
