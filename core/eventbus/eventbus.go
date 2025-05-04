package eventbus

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
)

type Event struct {
	ID        string
	Scope     string // "user", "system", "admin"
	Topic     string // e.g. "enqack/strategy.signal"
	Payload   map[string]string
	Source    string
	Broadcast bool
}

func NewEvent(scope, topic, source string, payload map[string]string, broadcast bool) Event {
	return Event{
		ID:        uuid.New().String(),
		Scope:     scope,
		Topic:     topic,
		Source:    source,
		Payload:   payload,
		Broadcast: broadcast,
	}
}

type Handler func(Event)

type Transport interface {
	PublishRemote(Event) error
	Broadcast(Event) error
	OnRemoteEvent(func(Event))
}

type EventBus struct {
	subs      map[string][]Handler
	transport Transport
	mu        sync.RWMutex
}

func NewEventBus(t Transport) *EventBus {
	bus := &EventBus{
		subs:      make(map[string][]Handler),
		transport: t,
	}

	if t != nil {
		t.OnRemoteEvent(func(e Event) {
			_ = bus.dispatch(e)
		})
	}

	return bus
}

var validScopes = map[string]bool{
	"user":   true,
	"system": true,
	"admin":  true,
}

func ValidateEvent(e Event) error {
	if !validScopes[e.Scope] {
		return fmt.Errorf("invalid scope: %s", e.Scope)
	}
	if strings.TrimSpace(e.Topic) == "" {
		return fmt.Errorf("topic cannot be empty")
	}
	return nil
}

func (bus *EventBus) Subscribe(scope string, topic string, fn Handler) error {
	if !validScopes[scope] {
		return fmt.Errorf("invalid scope: %s", scope)
	}
	fullKey := scope + "/" + topic
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subs[fullKey] = append(bus.subs[fullKey], fn)
	return nil
}

func (bus *EventBus) SubscribePrefix(scope string, topicPrefix string, fn Handler) error {
	if !validScopes[scope] {
		return fmt.Errorf("invalid scope: %s", scope)
	}
	key := "__MATCH:" + scope + "/" + topicPrefix
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subs[key] = append(bus.subs[key], fn)
	return nil
}

func (bus *EventBus) Publish(e Event) error {
	if err := ValidateEvent(e); err != nil {
		return err
	}

	if e.Broadcast && bus.transport != nil {
		_ = bus.transport.Broadcast(e)
	} else if bus.transport != nil {
		_ = bus.transport.PublishRemote(e)
	}

	return bus.dispatch(e)
}

func (bus *EventBus) dispatch(e Event) error {
	fullKey := e.Scope + "/" + e.Topic
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	if handlers, ok := bus.subs[fullKey]; ok {
		for _, fn := range handlers {
			go fn(e)
		}
	}

	for topic, handlers := range bus.subs {
		if strings.HasPrefix(topic, "__MATCH:") {
			base := strings.TrimPrefix(topic, "__MATCH:")
			if strings.HasPrefix(fullKey, base) {
				for _, fn := range handlers {
					go fn(e)
				}
			}
		}
	}

	return nil
}

type LocalTransport struct{}

func (t *LocalTransport) PublishRemote(e Event) error  { return nil }
func (t *LocalTransport) Broadcast(e Event) error      { return nil }
func (t *LocalTransport) OnRemoteEvent(fn func(Event)) {}
