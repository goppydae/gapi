package eventbus

type Transport interface {
	PublishRemote(Event) error
	Broadcast(Event) error
	OnRemoteEvent(func(Event))
}
