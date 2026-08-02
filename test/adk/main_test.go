package adk_test

import (
	"os"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// TestMain declares a product identity for this package's tests.
//
// Required, not decorative: the cross-ADK parity tests build a real
// supervisor.New in-process, which reaches cgroups.Setup and the config
// loader, and core/product has no usable default (GAPI-DIV-061). The
// harness that launches a real gapid needs nothing from this - that
// child declares itself - but the in-process half does.
func TestMain(m *testing.M) {
	product.Set("gapi")
	os.Exit(m.Run())
}
