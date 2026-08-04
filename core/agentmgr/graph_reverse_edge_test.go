// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package agentmgr

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/eventbus"
)

// reverseEdgeAgent builds a GoAgent with a live bus. A nil bus panics
// inside lifecycle.NewController, which would make these tests fail on
// a nil dereference rather than on the ordering claim they exist to
// make.
func reverseEdgeAgent(id string, wantedBy, requiredBy []string) Agent {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	return NewGoAgent(id, "svc", "/bin/true",
		nil, nil, wantedBy, requiredBy,
		"", "", "", nil, bus, nil)
}

// TestTopologicalSort_HonoursWantedBy pins the reverse-edge semantic that
// wanted_by has always claimed and never had.
//
// wanted_by is systemd's [Install] WantedBy=: "the named unit wants me",
// the reverse direction of Wants. It is parsed by discovery, validated by
// core/schema, stored on the concrete agent types, emitted in Describe(),
// and written into the registry graph as a reverse edge - and until this
// test it reached the ORDERING in neither of the two toposort callers,
// because both build hard/soft from Requires and Wants alone.
//
// The concrete failure that motivates it: an agent declaring
// wanted_by: ["some-target"] gets an edge in a query graph and no
// ordering effect at all, so anything built on wanted_by - a target
// model above all - silently orders nothing.
func TestTopologicalSort_HonoursWantedBy(t *testing.T) {
	// The names are deliberately anti-alphabetical: "zebra" must come
	// FIRST. With no edge between them toposort falls back to a
	// deterministic order, which is alphabetical, so an implementation
	// that drops wanted_by returns [alpha zebra] and fails here. Naming
	// them the other way round produced a test that passed against the
	// unfixed code - the tiebreak, not the edge, was doing the work.
	//
	// "zebra" declares that "alpha" wants it: the edge is alpha -> zebra,
	// so zebra orders first, exactly as if alpha had declared
	// wants: ["zebra"].
	agents := map[string]Agent{
		"zebra": reverseEdgeAgent("zebra", []string{"alpha"}, nil),
		"alpha": reverseEdgeAgent("alpha", nil, nil),
	}

	order, err := TopologicalSort(agents)
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	if !reflect.DeepEqual(order, []string{"zebra", "alpha"}) {
		t.Errorf("TopologicalSort() = %v, want [zebra alpha]: "+
			"zebra is wanted_by alpha, so it must be ordered first", order)
	}
}

// TestTopologicalSort_ForwardWantsOrdersAntiAlphabetically is the
// control. It states the same ordering with the FORWARD edge that
// already works, so a failure here means the tiebreak assumption above
// is wrong and the reverse-edge tests are measuring the wrong thing -
// not that reverse edges are broken.
func TestTopologicalSort_ForwardWantsOrdersAntiAlphabetically(t *testing.T) {
	bus := eventbus.NewInprocBus[*anypb.Any]()
	agents := map[string]Agent{
		"zebra": NewGoAgent("zebra", "svc", "/bin/true",
			nil, nil, nil, nil, "", "", "", nil, bus, nil),
		"alpha": NewGoAgent("alpha", "svc", "/bin/true",
			nil, []string{"zebra"}, nil, nil, "", "", "", nil, bus, nil),
	}

	order, err := TopologicalSort(agents)
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"zebra", "alpha"}) {
		t.Fatalf("control failed: TopologicalSort() = %v, want [zebra alpha]. "+
			"A forward wants edge does not order, so the reverse-edge tests "+
			"cannot be trusted", order)
	}
}

// TestTopologicalSort_HonoursRequiredBy is the hard-edge sibling.
// required_by is the reverse of Requires, so it must also cycle-reject
// rather than be silently dropped like a soft edge.
func TestTopologicalSort_HonoursRequiredBy(t *testing.T) {
	// Anti-alphabetical for the same reason as above: "zulu" must be first.
	agents := map[string]Agent{
		"zulu":  reverseEdgeAgent("zulu", nil, []string{"alpha"}),
		"alpha": reverseEdgeAgent("alpha", nil, nil),
	}

	order, err := TopologicalSort(agents)
	if err != nil {
		t.Fatalf("TopologicalSort() error = %v", err)
	}

	if !reflect.DeepEqual(order, []string{"zulu", "alpha"}) {
		t.Errorf("TopologicalSort() = %v, want [zulu alpha]: "+
			"zulu is required_by alpha, so it must be ordered first", order)
	}
}
