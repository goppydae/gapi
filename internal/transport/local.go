package transport

import "github.com/goppydae/gapi/core/eventbus"

type LocalTransport struct{}

func (t *LocalTransport) PublishRemote(e eventbus.Event) error  { return nil }
func (t *LocalTransport) Broadcast(e eventbus.Event) error      { return nil }
func (t *LocalTransport) OnRemoteEvent(fn func(eventbus.Event)) {}
