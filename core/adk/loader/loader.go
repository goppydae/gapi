package loader

import (
	"fmt"
	"path/filepath"

	"github.com/goppydae/gapi/core/adk"
	"github.com/goppydae/gapi/core/adk/loader/python"
)

// Load returns a fully functional agent that can be introspected and invoked.
func Load(path string) (adk.Agent, error) {
	switch ext := filepath.Ext(path); ext {
	case ".py":
		return python.Load(path)
	default:
		return nil, fmt.Errorf("unsupported agent file type: %s", ext)
	}
}
