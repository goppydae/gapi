package eventbus

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"
)

func TestUnsubscribePrefix(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer bus.Close()

	callCount := 0
	handler := func(e Event[*anypb.Any]) {
		callCount++
	}

	// Subscribe
	err := bus.SubscribePrefix("system", "", "test", handler)
	if err != nil {
		t.Fatalf("SubscribePrefix failed: %v", err)
	}

	// Publish an event - should be received
	ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil, false)
	if err := bus.Publish(ev); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Wait for handler to execute
	time.Sleep(50 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("Expected callCount=1 before unsubscribe, got %d", callCount)
	}

	// Unsubscribe
	bus.UnsubscribePrefix("system", "", "test", handler)

	// Publish another event - should NOT be received
	ev2 := NewEvent[*anypb.Any]("system", "", "test.msg2", "test", nil, false)
	if err := bus.Publish(ev2); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Wait to ensure no handler execution
	time.Sleep(50 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("Expected callCount=1 after unsubscribe, got %d", callCount)
	}
}

func TestSubscribePrefixWithContext_Cleanup(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer bus.Close()

	callCount := 0
	handler := func(e Event[*anypb.Any]) {
		callCount++
	}

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Subscribe with context
	err := bus.SubscribePrefixWithContext(ctx, "system", "", "test", handler)
	if err != nil {
		t.Fatalf("SubscribePrefixWithContext failed: %v", err)
	}

	// Publish an event - should be received
	ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil, false)
	if err := bus.Publish(ev); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("Expected callCount=1 before context cancel, got %d", callCount)
	}

	// Cancel the context
	cancel()

	// Wait for cleanup goroutine to execute
	time.Sleep(50 * time.Millisecond)

	// Publish another event - should NOT be received
	ev2 := NewEvent[*anypb.Any]("system", "", "test.msg2", "test", nil, false)
	if err := bus.Publish(ev2); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if callCount != 1 {
		t.Errorf("Expected callCount=1 after context cancel, got %d (subscription should be cleaned up)", callCount)
	}
}

func TestSubscribePrefixWithContext_MultipleSubscribers(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer bus.Close()

	count1, count2 := 0, 0
	handler1 := func(e Event[*anypb.Any]) { count1++ }
	handler2 := func(e Event[*anypb.Any]) { count2++ }

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
	ev := NewEvent[*anypb.Any]("system", "", "test.msg", "test", nil, false)
	if err := bus.Publish(ev); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if count1 != 1 || count2 != 1 {
		t.Errorf("Expected both handlers to be called once, got count1=%d, count2=%d", count1, count2)
	}

	// Cancel first context only
	cancel1()
	time.Sleep(50 * time.Millisecond)

	// Publish again - only handler2 should receive
	ev2 := NewEvent[*anypb.Any]("system", "", "test.msg2", "test", nil, false)
	if err := bus.Publish(ev2); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if count1 != 1 {
		t.Errorf("Expected handler1 count to remain 1, got %d", count1)
	}
	if count2 != 2 {
		t.Errorf("Expected handler2 count to be 2, got %d", count2)
	}
}
