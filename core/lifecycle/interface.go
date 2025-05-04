package lifecycle

type Daemon interface {
	ID() string
	Scope() string
	Type() string
	Start() error
	Stop() error
	Reload() error
	Describe() map[string]string
}
