package agent

import "encoding/json"

// SchemaVersion matches the Python runner's SCHEMA_VERSION. The two ADKs
// describe into ONE schema because discovery has one decoder for both
// (core/agentmgr's pyDescribe), so a version that drifts here is a parity
// break rather than a Go-side detail.
const SchemaVersion = "1.0.0"

// describePayload is the --describe wire form. Every key the Python
// runner emits appears here, with the same spelling and the same type,
// including the ones a Go agent has historically omitted: requires,
// wants, wanted_by, required_by, enabled, cpu_limit and memory_limit. A
// scaffolded Go agent could not express dependencies or resource limits
// at all while those were missing (GAPI-DIV-051).
type describePayload struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Type          string `json:"type"`
	Language      string `json:"language"`
	Enabled       bool   `json:"enabled"`
	Description   string `json:"description"`

	Capabilities []string `json:"capabilities"`
	Requires     []string `json:"requires"`
	Wants        []string `json:"wants"`
	WantedBy     []string `json:"wanted_by"`
	RequiredBy   []string `json:"required_by"`

	ListenStream string `json:"listen_stream"`
	CPULimit     string `json:"cpu_limit"`
	MemoryLimit  string `json:"memory_limit"`
	Schedule     string `json:"schedule"`
}

type describeEnvelope struct {
	Describe describePayload `json:"describe"`
}

// capabilities lists the lifecycle functions this agent actually has, in
// the Python runner's declaration order so the two languages produce
// identical arrays for identical agents - the cross-ADK suite compares
// them element by element, not as sets.
//
// Declared extras follow the lifecycle set and are de-duplicated, which
// is the order and the treatment runner.py gives its @capability
// decorators - lifecycle names first, decorated names appended, then
// dict.fromkeys to dedupe.
func (s *Spec) capabilities() []string {
	caps := []string{}
	for _, c := range []struct {
		name    string
		present bool
	}{
		{"initialize", s.Initialize != nil},
		{"start", s.Start != nil},
		{"stop", s.Stop != nil},
		{"reload", s.Reload != nil},
		{"restart", s.Restart != nil},
	} {
		if c.present {
			caps = append(caps, c.name)
		}
	}

	seen := map[string]bool{}
	for _, c := range caps {
		seen[c] = true
	}
	for _, c := range s.Capabilities {
		if c != "" && !seen[c] {
			seen[c] = true
			caps = append(caps, c)
		}
	}
	return caps
}

// describe renders the payload.
//
// Slices are normalised to empty rather than nil so the JSON carries []
// where Python carries []. A nil slice marshals to null, and null is not
// the same value as an empty list to anything that reads this - which is
// the sort of difference the parity contract exists to keep out.
func (s *Spec) describe() describeEnvelope {
	enabled := true
	if s.Enabled != nil {
		enabled = *s.Enabled
	}

	name := s.Name
	if name == "" {
		name = s.ID
	}

	return describeEnvelope{Describe: describePayload{
		SchemaVersion: SchemaVersion,
		ID:            s.ID,
		Name:          name,
		Version:       s.Version,
		Type:          s.Type,
		Language:      "go",
		Enabled:       enabled,
		Description:   s.Description,
		Capabilities:  s.capabilities(),
		Requires:      orEmpty(s.Requires),
		Wants:         orEmpty(s.Wants),
		WantedBy:      orEmpty(s.WantedBy),
		RequiredBy:    orEmpty(s.RequiredBy),
		ListenStream:  s.ListenStream,
		CPULimit:      s.CPULimit,
		MemoryLimit:   s.MemoryLimit,
		Schedule:      s.Schedule,
	}}
}

func orEmpty(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// describeJSON renders the describe envelope as the single line
// discovery's binaryDescribe parses from the child's stdout.
func (s *Spec) describeJSON() ([]byte, error) {
	return json.Marshal(s.describe())
}
