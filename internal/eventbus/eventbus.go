package eventbus

import (
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/goppydae/gapi/internal/logging/logcore"
	"github.com/goppydae/gapi/internal/logging/logevent"
)

// Event is now generic over the payload type T.
// If you still use protobuf Any, instantiate as Event[*anypb.Any].
type Event[T any] struct {
	ID        string
	Scope     string // "user", "system", "admin"
	Topic     string // e.g. "enqack/strategy.signal"
	Payload   T
	Source    string
	Broadcast bool
}

func NewEvent[T any](scope, topic, source string, payload T, broadcast bool) Event[T] {
	return Event[T]{
		ID:        uuid.New().String(),
		Scope:     scope,
		Topic:     topic,
		Source:    source,
		Payload:   payload,
		Broadcast: broadcast,
	}
}

// Handler is typed on the same T as Event.
type Handler[T any] func(Event[T])

// Transport is generic and injected (see interface.go).
type EventBus[T any] struct {
	subs      map[string][]Handler[T]
	transport Transport[T]
	mu        sync.RWMutex
}

func NewEventBus[T any](t Transport[T]) *EventBus[T] {
	bus := &EventBus[T]{
		subs:      make(map[string][]Handler[T]),
		transport: t,
	}

	if t != nil {
		t.OnRemoteEvent(func(e Event[T]) {
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

func ValidateEvent[T any](e Event[T]) error {
	if !validScopes[e.Scope] {
		return fmt.Errorf("invalid scope: %s", e.Scope)
	}
	if strings.TrimSpace(e.Topic) == "" {
		return fmt.Errorf("topic cannot be empty")
	}
	return nil
}

func (bus *EventBus[T]) Subscribe(scope, topic string, fn Handler[T]) error {
	if !validScopes[scope] {
		return fmt.Errorf("invalid scope: %s", scope)
	}
	fullKey := scope + "/" + topic
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subs[fullKey] = append(bus.subs[fullKey], fn)
	return nil
}

func (bus *EventBus[T]) SubscribePrefix(scope, topicPrefix string, fn Handler[T]) error {
	if !validScopes[scope] {
		return fmt.Errorf("invalid scope: %s", scope)
	}
	key := "__MATCH:" + scope + "/" + topicPrefix
	bus.mu.Lock()
	defer bus.mu.Unlock()
	bus.subs[key] = append(bus.subs[key], fn)
	return nil
}

func (bus *EventBus[T]) SubscribeOnce(scope, topic string, handler Handler[T]) {
	var once sync.Once
	var wrapper Handler[T]

	wrapper = func(e Event[T]) {
		once.Do(func() {
			handler(e)
			bus.Unsubscribe(scope, topic, wrapper)
		})
	}

	_ = bus.Subscribe(scope, topic, wrapper)
}

func (bus *EventBus[T]) Unsubscribe(scope, topic string, target Handler[T]) {
	key := fmt.Sprintf("%s/%s", scope, topic)

	bus.mu.Lock()
	defer bus.mu.Unlock()

	handlers := bus.subs[key]
	for i, h := range handlers {
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", target) {
			bus.subs[key] = append(handlers[:i], handlers[i+1:]...)
			break
		}
	}

	if len(bus.subs[key]) == 0 {
		delete(bus.subs, key)
	}
}

func (bus *EventBus[T]) Publish(e Event[T]) error {
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

func (bus *EventBus[T]) dispatch(e Event[T]) error {
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

// LocalTransport is a no-op in-proc transport; kept generic for testing.
type LocalTransport[T any] struct{}

func (t *LocalTransport[T]) PublishRemote(e Event[T]) error  { return nil }
func (t *LocalTransport[T]) Broadcast(e Event[T]) error      { return nil }
func (t *LocalTransport[T]) OnRemoteEvent(fn func(Event[T])) {}

// Back-compat helper: when T == *anypb.Any you can still unmarshal.
func (e *Event[T]) UnmarshalPayload(target proto.Message) error {
	anyPayload, ok := any(e.Payload).(*anypb.Any)
	if !ok {
		return fmt.Errorf("payload is not *anypb.Any")
	}
	if anyPayload == nil {
		return fmt.Errorf("event has no payload")
	}
	return anyPayload.UnmarshalTo(target)
}
