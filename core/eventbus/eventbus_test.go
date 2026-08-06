// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
)

// Handlers run in bus-spawned goroutines (dispatch uses `go fn(e)`), so
// test counters they touch must be atomic; a plain int read from the test
// goroutine races the handler's write.

func TestUnsubscribePrefix(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	}()

	var callCount atomic.Int32
	handler := func(e Event[*anypb.Any]) {
		callCount.Add(1)
	}

	// Subscribe
	err := bus.SubscribePrefix("system", "", "test", handler)
	if err != nil {
		t.Fatalf("SubscribePrefix failed: %v", err)
	}

	// Publish an event - should be received
	ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil)
	if err := bus.Publish(ev); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Wait for handler to execute
	time.Sleep(50 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("Expected callCount=1 before unsubscribe, got %d", n)
	}

	// Unsubscribe
	bus.UnsubscribePrefix("system", "", "test", handler)

	// Publish another event - should NOT be received
	ev2 := NewEvent[*anypb.Any]("system", "", "test.msg2", "test", nil)
	if err := bus.Publish(ev2); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Wait to ensure no handler execution
	time.Sleep(50 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("Expected callCount=1 after unsubscribe, got %d", n)
	}
}

func TestSubscribePrefixWithContext_Cleanup(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	}()

	var callCount atomic.Int32
	handler := func(e Event[*anypb.Any]) {
		callCount.Add(1)
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Subscribe with context
	err := bus.SubscribePrefixWithContext(ctx, "system", "", "test", handler)
	if err != nil {
		t.Fatalf("SubscribePrefixWithContext failed: %v", err)
	}

	// Publish an event - should be received
	ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil)
	if err := bus.Publish(ev); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("Expected callCount=1 before context cancel, got %d", n)
	}

	// Cancel the context
	cancel()

	// Wait for cleanup goroutine to execute
	time.Sleep(50 * time.Millisecond)

	// Publish another event - should NOT be received
	ev2 := NewEvent[*anypb.Any]("system", "", "test.msg2", "test", nil)
	if err := bus.Publish(ev2); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if n := callCount.Load(); n != 1 {
		t.Errorf("Expected callCount=1 after context cancel, got %d (subscription should be cleaned up)", n)
	}
}

func TestSubscribePrefixWithContext_MultipleSubscribers(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	}()

	var count1, count2 atomic.Int32
	handler1 := func(e Event[*anypb.Any]) { count1.Add(1) }
	handler2 := func(e Event[*anypb.Any]) { count2.Add(1) }

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()

	// Subscribe both
	if err := bus.SubscribePrefixWithContext(ctx1, "system", "", "test", handler1); err != nil {
		t.Fatalf("SubscribePrefixWithContext failed: %v", err)
	}
	if err := bus.SubscribePrefixWithContext(ctx2, "system", "", "test", handler2); err != nil {
		t.Fatalf("SubscribePrefixWithContext failed: %v", err)
	}

	// Publish - both should receive
	ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil)
	if err := bus.Publish(ev); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if count1.Load() != 1 || count2.Load() != 1 {
		t.Errorf("Expected both handlers to be called once, got count1=%d, count2=%d", count1.Load(), count2.Load())
	}

	// Cancel first context only
	cancel1()
	time.Sleep(50 * time.Millisecond)

	// Publish again - only handler2 should receive
	ev2 := NewEvent[*anypb.Any]("system", "", "test.msg2", "test", nil)
	if err := bus.Publish(ev2); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if n := count1.Load(); n != 1 {
		t.Errorf("Expected handler1 count to remain 1, got %d", n)
	}
	if n := count2.Load(); n != 2 {
		t.Errorf("Expected handler2 count to be 2, got %d", n)
	}
}
