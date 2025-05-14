package lifecycle

// Agent represents the lifecycle of any managed external program or script.
type Agent interface {
	ID() string                  // Unique agent ID (derived from filename)
	Scope() string               // Namespace or scope (e.g., system, admin)
	Type() string                // Agent type: service, timer, etc.
	Start() error                // Boot the agent
	Stop() error                 // Halt the agent
	Restart() error              // Stop, then start
	Reload() error               // Soft restart (reconfiguration or reinitialization)
	Describe() map[string]string // Describe basic agent metadata
}
