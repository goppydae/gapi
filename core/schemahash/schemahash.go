// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// Package schemahash computes the identity of the protobuf contract a
// binary was compiled against.
//
// ONE IMPLEMENTATION, CALLED BY BOTH SIDES. The agent computes it
// through adk/go and the daemon through core/version, and if they
// computed it separately they could drift into hashing different
// things - which is GAPI-DIV-127's third defect in a new costume.
// Neither of those packages can import the other, so this is a leaf
// both depend on.
//
// WHAT THIS IS NOT. The value answers "was this built from the same
// contract sources", NOT "can these two safely talk". Protobuf exists so
// those differ: adding a field changes this hash and breaks nothing.
// Operator decision 71 therefore makes it a diagnostic and never an
// enforcement input - the daemon reports skew and never refuses an
// agent. See GAPI-DIV-127.
package schemahash

import (
	"fmt"
	"sort"
	"sync"

	"github.com/zeebo/blake3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// gapiPackage is the only protobuf package whose descriptors describe
// the agent-to-daemon contract.
//
// SCOPED RATHER THAN HASHING EVERY LINKED FILE. A binary that happens to
// link an unrelated protobuf dependency would otherwise report skew
// against one that does not, and the two would still agree perfectly
// about the contract they share.
const gapiPackage = "gapi.v1"

// fileRanger is the seam the tests drive. protoregistry.GlobalFiles
// satisfies it.
//
// It exists for the same reason core/version's buildInfoReader does: a
// resolution reachable only through a global is a resolution no test can
// give a second input, and a hash with one input cannot be shown to
// discriminate.
type fileRanger interface {
	RangeFiles(func(protoreflect.FileDescriptor) bool)
}

var (
	once   sync.Once
	cached string
)

// Contract returns the hex BLAKE3 digest of the linked gapi.v1
// descriptor set. Computed once; stable for the life of the process.
func Contract() string {
	once.Do(func() { cached = contractFrom(protoregistry.GlobalFiles) })
	return cached
}

// contractFrom hashes one descriptor source.
//
// SORTED BY PATH BEFORE HASHING, which is load-bearing rather than tidy:
// RangeFiles promises no ordering, so an order-sensitive digest would
// report skew between two byte-identical binaries - a detector whose
// false positives arrive first and train everyone to ignore it.
// Marshalled with Deterministic so map-valued options cannot reorder
// either.
func contractFrom(files fileRanger) string {
	var paths []string
	byPath := map[string]protoreflect.FileDescriptor{}
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		if string(fd.Package()) != gapiPackage {
			return true
		}
		p := fd.Path()
		paths = append(paths, p)
		byPath[p] = fd
		return true
	})
	sort.Strings(paths)

	h := blake3.New()
	marshal := proto.MarshalOptions{Deterministic: true}
	for _, p := range paths {
		// The path is hashed too, so moving a file between packages is a
		// change even when its contents are byte-identical.
		_, _ = h.WriteString(p)
		raw, err := marshal.Marshal(protodesc.ToFileDescriptorProto(byPath[p]))
		if err != nil {
			// A descriptor that cannot marshal is a broken build, not a
			// skew. Fold the error into the digest rather than skipping
			// it, so a binary in that state cannot silently produce the
			// same value as a healthy one.
			_, _ = h.WriteString("unmarshalable:" + err.Error())
			continue
		}
		_, _ = h.Write(raw)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
