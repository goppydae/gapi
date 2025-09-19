package loader

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/goppydae/gapi/core/adk"
	"github.com/goppydae/gapi/core/adk/loader/python"
)

// Load returns a fully functional agent that can be introspected and invoked.
func Load(path string) (adk.Agent, error) {
	base := filepath.Base(path)
	parts := strings.Split(base, ".")

	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid agent filename: expected format <name>.<lang>.<type>, got %s", base)
	}

	lang := parts[len(parts)-2]
	typ := parts[len(parts)-1]

	log.Printf("Loading agent file %s (lang=%s, type=%s)", base, lang, typ)

	switch lang {
	case "py":
		return python.Load(path)
	default:
		return nil, fmt.Errorf("unsupported agent language: %s", lang)
	}
}
