package logevent

type LifecyclePayload struct {
	Action   string `json:"action"` // e.g., "start", "stop", "reload"
	DaemonID string `json:"daemon_id"`
	Version  string `json:"version,omitempty"`
}
