package eventbus

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/core/clock"
)

func TestWaitForTopic_EventArrives(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- bus.WaitForTopic(context.Background(), "system", "", TopicAgentNetworkRunning, 5*time.Second, clock.RealClock{})
	}()

	// Publish until the waiter reports; the subscription races the publish,
	// so retry rather than sleep-and-hope.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("WaitForTopic = %v, want nil", err)
			}
			return
		case <-deadline:
			t.Fatal("WaitForTopic did not observe the published event")
		default:
			ev := NewEvent[*anypb.Any]("system", "", TopicAgentNetworkRunning, "test", nil, false)
			if err := bus.Publish(ev); err != nil {
				t.Fatalf("Publish: %v", err)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestWaitForTopic_TimesOut(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	}()

	// MockClock.After fires immediately: the deterministic timeout path.
	err := bus.WaitForTopic(context.Background(), "system", "", TopicAgentNetworkRunning, 120*time.Second, &clock.MockClock{})
	if !errors.Is(err, ErrWaitTimeout) {
		t.Fatalf("WaitForTopic = %v, want ErrWaitTimeout", err)
	}
}

func TestWaitForTopic_ContextCancel(t *testing.T) {
	bus := NewInprocBus[*anypb.Any]()
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("close bus: %v", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := bus.WaitForTopic(ctx, "system", "", TopicAgentNetworkRunning, time.Minute, clock.RealClock{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitForTopic = %v, want context.Canceled", err)
	}
}
