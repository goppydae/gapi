package transport

import "github.com/goppydae/gapi/core/eventbus"

type Transport interface {
	PublishRemote(eventbus.Event) error
	Broadcast(eventbus.Event) error
	OnRemoteEvent(func(eventbus.Event))
}
