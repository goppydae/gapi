package logging_test

import (
	"os"
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// TestMain declares a product identity for this package's tests: the
// kmsg tag is composed from it, and core/product has no usable default
// (GAPI-DIV-061).
func TestMain(m *testing.M) {
	product.Set("gapi")
	os.Exit(m.Run())
}
