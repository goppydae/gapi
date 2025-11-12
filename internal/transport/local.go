package transport

import "github.com/goppydae/gapi/internal/eventbus"

type Local[T any] struct {
	onRemote func(eventbus.Event[T])
}

func (t *Local[T]) PublishRemote(e eventbus.Event[T]) error {
	// Loop back as if it arrived remotely.
	if t.onRemote != nil {
		t.onRemote(e)
	}
	return nil
}

func (t *Local[T]) Broadcast(e eventbus.Event[T]) error {
	// Same semantics as PublishRemote for Local.
	return t.PublishRemote(e)
}

func (t *Local[T]) OnRemoteEvent(fn func(eventbus.Event[T])) { t.onRemote = fn }

func (t *Local[T]) Close() error { return nil }
