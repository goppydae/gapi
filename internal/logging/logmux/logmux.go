package logmux

import (
	"io"
	"sync"
)

type MuxWriter struct {
	writers []io.Writer
	mu      sync.Mutex
}

// New creates a writer that duplicates to all outputs.
func New(writers ...io.Writer) *MuxWriter {
	return &MuxWriter{writers: writers}
}

// Write writes to all underlying writers.
func (m *MuxWriter) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, w := range m.writers {
		if _, err = w.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
