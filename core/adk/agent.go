package adk

import "github.com/goppydae/gapi/core/adk/meta"

type Agent interface {
	Info() *meta.AgentInfo    // Return metadata
	Call(fnName string) error // Call a lifecycle function like "start"
}
