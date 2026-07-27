package eventbus

import (
	"testing"
	"time"
)

// #15: SubscribeCorrelated must deliver a reply only to the waiter whose
// correlation ID matches, so concurrent callers sharing one reply topic don't
// steal each other's responses.
func TestSubscribeCorrelated_FiltersByID(t *testing.T) {
	bus := NewInprocBus[string]()

	got := make(chan string, 4)
	if err := bus.SubscribeCorrelated("system", "", "reply", "corr-A", func(e Event[string]) {
		got <- "A:" + e.Payload
	}); err != nil {
		t.Fatalf("subscribe A: %v", err)
	}
	if err := bus.SubscribeCorrelated("system", "", "reply", "corr-B", func(e Event[string]) {
		got <- "B:" + e.Payload
	}); err != nil {
		t.Fatalf("subscribe B: %v", err)
	}

	// Publish B's reply first, then A's — each waiter must receive only its own.
	evB := NewEvent("system", "", "reply", "srv", "payloadB", false)
	evB.ID = "corr-B"
	if err := bus.Publish(evB); err != nil {
		t.Fatalf("publish B: %v", err)
	}
	evA := NewEvent("system", "", "reply", "srv", "payloadA", false)
	evA.ID = "corr-A"
	if err := bus.Publish(evA); err != nil {
		t.Fatalf("publish A: %v", err)
	}

	results := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case r := <-got:
			results[r] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out; got so far: %v", results)
		}
	}
	if !results["A:payloadA"] || !results["B:payloadB"] {
		t.Fatalf("correlation mismatch, got: %v", results)
	}

	// No extra deliveries (each correlated sub fires at most once).
	select {
	case extra := <-got:
		t.Fatalf("unexpected extra delivery: %q", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

// Subscriptions created at the same code site are distinct closure instances
// sharing one code pointer; self-unsubscribe must remove exactly its own
// entry, never another caller's. Regression: identity by fmt %p (the code
// pointer) made the second caller's unsubscribe remove the first caller's
// live subscription, silently losing its reply.
func TestSubscribeCorrelated_SameCallsiteCallersKeepOwnSubscriptions(t *testing.T) {
	bus := NewInprocBus[string]()
	defer bus.Close()

	subscribe := func(corrID string, got chan struct{}) {
		t.Helper()
		if err := bus.SubscribeCorrelated("system", "", "reply", corrID, func(Event[string]) {
			close(got)
		}); err != nil {
			t.Fatalf("SubscribeCorrelated(%s): %v", corrID, err)
		}
	}

	gotA := make(chan struct{})
	gotB := make(chan struct{})
	subscribe("id-A", gotA) // registered first
	subscribe("id-B", gotB) // registered second

	// Fire B first: B's self-unsubscribe runs while A is still waiting.
	evB := NewEvent[string]("system", "", "reply", "test", "pong", false)
	evB.ID = "id-B"
	if err := bus.Publish(evB); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gotB:
	case <-time.After(2 * time.Second):
		t.Fatal("B never received its reply")
	}
	time.Sleep(50 * time.Millisecond) // let B's self-unsubscribe land

	evA := NewEvent[string]("system", "", "reply", "test", "pong", false)
	evA.ID = "id-A"
	if err := bus.Publish(evA); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gotA:
	case <-time.After(2 * time.Second):
		t.Fatal("A's subscription was removed by B's self-unsubscribe (handler-identity bug)")
	}
}
