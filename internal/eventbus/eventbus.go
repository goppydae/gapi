package eventbus

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/internal/logging/logcore"
	"github.com/goppydae/gapi/internal/logging/logevent"
)

type Event struct {
	ID        string
	Scope     string // "user", "system", "admin"
	Topic     string // e.g. "enqack/strategy.signal"
	Payload   *anypb.Any
	Source    string
	Broadcast bool
}

func NewEvent(scope, topic, source string, payload *anypb.Any, broadcast bool) Event {
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
		logcore.Warn().
			Str("event", "reject").
			Str("event_id", e.ID).
			Str("topic", e.Topic).
			Str("scope", e.Scope).
			Msg("rejected invalid event")
		return err
	}

	logevent.Log(logcore.With().Str("module", "eventbus").Logger(), logevent.Event{
		ID:     e.ID,
		Type:   "publish",
		Source: e.Source,
		Payload: logevent.BusPayload{
			Topic:   e.Topic,
			Payload: fmt.Sprintf("%v", e.Payload),
		},
	})

	if e.Broadcast && bus.transport != nil {
		if err := bus.transport.Broadcast(e); err != nil {
			logcore.Error().
				Err(err).
				Str("event_id", e.ID).
				Str("topic", e.Topic).
				Msg("transport.Broadcast failed")
		}
	} else if bus.transport != nil {
		if err := bus.transport.PublishRemote(e); err != nil {
			logcore.Error().
				Err(err).
				Str("event_id", e.ID).
				Str("topic", e.Topic).
				Msg("transport.PublishRemote failed")
		}
	} else {
		logcore.Warn().
			Str("event_id", e.ID).
			Str("topic", e.Topic).
			Msg("no transport available for publish")
	}

	return bus.dispatch(e)
}

func (bus *EventBus) dispatch(e Event) error {
	fullKey := e.Scope + "/" + e.Topic

	logevent.Log(logcore.With().Str("module", "eventbus").Logger(), logevent.Event{
		ID:     e.ID,
		Type:   "dispatch",
		Source: e.Source,
		Payload: logevent.BusPayload{
			Topic:   e.Topic,
			Payload: fmt.Sprintf("%v", e.Payload),
		},
	})

	bus.mu.RLock()
	defer bus.mu.RUnlock()

	if handlers, ok := bus.subs[fullKey]; ok {
		for _, fn := range handlers {
			go fn(e)
		}
	}

	logcore.Info().
		Str("dispatch_key", fullKey).
		Int("subs_len", len(bus.subs)).
		Msg("checking topic subscriptions for prefix match")

	for topic := range bus.subs {
		logcore.Info().Str("sub_key", topic).Msg("registered subscription")
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
