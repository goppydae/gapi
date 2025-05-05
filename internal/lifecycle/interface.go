package lifecycle

type Daemon interface {
	ID() string
	Scope() string
	Type() string
	Start() error
	Stop() error
	Restart() error
	Reload() error
	Describe() map[string]string
}
