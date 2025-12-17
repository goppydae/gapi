package transport

import (
	"context"
	"testing"
	"time"

	"github.com/goppydae/gapi/core/eventbus"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestQUICHandshake(t *testing.T) {
	addr := "127.0.0.1:40001"

	cert, err := GenerateInsecureSelfSignedCert()
	if err != nil {
		t.Fatalf("Failed to generate cert: %v", err)
	}

	server, err := NewQUICServer(addr, cert)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Close()

	received := make(chan eventbus.Event[*anypb.Any], 1)
	server.OnRemoteEvent(func(e eventbus.Event[*anypb.Any]) {
		received <- e
	})

	client, err := NewQUICClient(addr, nil, TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer client.Close()

	// Wait for connection to stabilize
	time.Sleep(100 * time.Millisecond)

	testEvent := eventbus.Event[*anypb.Any]{
		ID:    "test-123",
		Topic: "test/topic",
	}

	if err := client.PublishRemote(context.Background(), testEvent); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	select {
	case ev := <-received:
		if ev.ID != testEvent.ID {
			t.Errorf("Expected event ID %s, got %s", testEvent.ID, ev.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}
