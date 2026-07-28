package loader

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/goppydae/gapi/core/adk"
	"github.com/goppydae/gapi/core/adk/loader/python"
	"github.com/goppydae/gapi/internal/logattr"
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

	slog.Default().LogAttrs(context.Background(), slog.LevelInfo, "loading agent file", logattr.Path(base), logattr.Lang(lang), logattr.Type(typ))

	switch lang {
	case "py":
		return python.Load(path)
	default:
		return nil, fmt.Errorf("unsupported agent language: %s", lang)
	}
}
