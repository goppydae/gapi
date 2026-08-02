package config

import (
	"os"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// TestMain declares a product identity for this package's tests.
//
// It is required rather than convenient. core/product has no usable
// default (GAPI-DIV-061), so config.Load, EnvKeyFor and AgentSearchPaths
// panic until a binary says who it is - and a test binary is a binary
// with no root command to say it. Setting "gapi" here is what a gapid
// process does at startup; the tests then exercise the same code path a
// real gapid takes.
func TestMain(m *testing.M) {
	product.Set("gapi")
	os.Exit(m.Run())
}
