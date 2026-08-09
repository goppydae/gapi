// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agent

import (
	"bytes"
	"testing"

	"github.com/goppydae/gapi/core/schemahash"
	gapiv1 "github.com/goppydae/gapi/pkg/proto"
	"google.golang.org/protobuf/encoding/protodelim"
)

// TestStatusCarriesTheContractHash closes GAPI-DIV-127's second defect
// where it recurred.
//
// The Python path sets it - adk/go/core.go's SendEvent has since Task 2 -
// and THIS path, the one Go agents actually use, did not. A field one
// language populates and the other does not is the exact state that
// entry was filed against, and it had reappeared one message down.
func TestStatusCarriesTheContractHash(t *testing.T) {
	var buf bytes.Buffer
	c := newControlTo(&buf, "probe")

	c.status("RUNNING", "up")

	var frame gapiv1.AgentControl
	if err := protodelim.UnmarshalFrom(bytes.NewReader(buf.Bytes()), &frame); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	got := frame.GetStatus().GetSchemaHash()
	if got == "" {
		t.Fatal("a Go agent's status carries no contract hash")
	}
	if want := schemahash.Contract(); got != want {
		t.Fatalf("status hash %q does not match the linked contract %q", got, want)
	}
}
