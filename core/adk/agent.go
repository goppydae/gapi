package adk

import "github.com/goppydae/gapi/core/adk/meta"

// Agent is the standard contract for all supervised agents
type Agent interface {
	Initialize() error
	Start() error
	Stop() error
	Describe() *meta.AgentInfo
	Info() *meta.AgentInfo
}

// OptionalHooks can be optionally implemented by agents
type OptionalHooks interface {
	Restart() error
	Reload() error
}
