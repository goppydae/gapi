// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"testing"

	"github.com/goppydae/gapi/core/product"
)

// GAPI-DIV-061's third gate: each binary's root declares the product it
// belongs to, and declares the RIGHT one.
//
// Asserting only "an identity was set" would pass for a root that named
// the wrong product, which is the residual recorded on the entry. These
// assert the value. What they cannot cover is goblin's two roots, which
// live in the other repo - GOBLIN-DIV-055 carries the same assertion
// there.

func TestGapidRootDeclaresTheProduct(t *testing.T) {
	product.Set("goblin") // a wrong value the constructor must overwrite
	if _, _, _ = NewGapidRoot(noopStart); product.Name() != "gapi" {
		t.Errorf("after NewGapidRoot, product = %q, want %q", product.Name(), "gapi")
	}
}

func TestGapictlRootDeclaresTheProduct(t *testing.T) {
	product.Set("goblin")
	if _, _ = NewGapictlRoot(); product.Name() != "gapi" {
		t.Errorf("after NewGapictlRoot, product = %q, want %q", product.Name(), "gapi")
	}
}

// The package-level gapictl singleton must NOT declare an identity.
//
// This is the ordering trap, and it is the one assertion here that
// guards a property nothing else can see. pkg/cli is initialized inside
// goblind and goblinctl too; if that initializer claimed "gapi", every
// embedder would boot with a working default for a value required to
// have none, and core/product's panic would be dead code for the only
// callers it exists to protect.
//
// It runs by construction: the singleton is built during package
// initialization, long before any test body.
func TestPackageInitDoesNotDeclareAnIdentity(t *testing.T) {
	if !initSawUnsetIdentity {
		t.Error("pkg/cli's package initialization declared a product identity. " +
			"That initializer runs inside goblind and goblinctl, so every embedder " +
			"would silently inherit gapi's namespace and the no-default guarantee " +
			"would never fire (GAPI-DIV-061). Build the singleton with " +
			"newControlTree, not NewControlRoot.")
	}
	if rootCmd == nil || controlFlags == nil {
		t.Fatal("the singleton was not built at package initialization")
	}
}

// initSawUnsetIdentity records what the identity looked like immediately
// after this package's variable initializers ran. It is a var rather
// than a check inside the test because by test time any other test may
// have set one.
//
// Declared after rootCmd in the package's initialization order? No - Go
// orders initializers by dependency, and this one depends on nothing, so
// source order across files is not something to rely on. It reads
// product.IsSet() through a function call that depends on rootCmd,
// forcing the singleton to be built first.
var initSawUnsetIdentity = identityUnsetAfter(rootCmd)

func identityUnsetAfter(built any) bool {
	_ = built // the parameter exists to order this after the singleton
	return !product.IsSet()
}
