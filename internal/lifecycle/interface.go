package lifecycle

import "github.com/goppydae/gapi/core/adk/meta"

type Agent interface {
	Initialize() error
	Start() error
	Stop() error
	Restart() error
	Reload() error
	Describe() *meta.AgentInfo
	ID() string
	Type() string
	Scope() string
}
