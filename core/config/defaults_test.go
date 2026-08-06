// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// leafPaths returns every scalar config path, reusing the reflection
// walk that envoverride_test.go already established as the way to
// enumerate this tree: a field added later is covered because it exists,
// not because someone remembered to list it.
func leafPaths() []string {
	var out []string
	for _, l := range leaves(reflect.TypeOf(Config{}), "", nil) {
		out = append(out, l.path)
	}
	sort.Strings(out)
	return out
}

// defaultsOnly builds a viper carrying ONLY the registered defaults - no
// environment binding, no AutomaticEnv, no config file - so that
// IsSet is answering about defaults and nothing else.
func defaultsOnly() *viper.Viper {
	v := viper.New()
	setDefaults(v)
	return v
}

// Defaults must describe the whole schema, in both directions.
//
// This is the check that could not be written before: twelve of the
// thirty-three reachable keys had no SetDefault, so a key present in the
// struct and absent from the defaults was indistinguishable from a key
// nobody had gotten to yet. The generated configuration reference is
// produced by joining these two walks, and a join over two sets that are
// allowed to disagree documents whichever one it happened to read.
func TestDefaultsRegistersEveryReachableKey(t *testing.T) {
	v := defaultsOnly()

	paths := leafPaths()
	if len(paths) < 20 {
		t.Fatalf("found only %d config leaves; the walk is not reaching the tree", len(paths))
	}

	var missing []string
	for _, p := range paths {
		if !v.IsSet(p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d config keys reachable from the struct have no default:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// The reverse direction: a default for a key the struct does not have is
// a value that can never be unmarshalled into anything, and it would
// appear in the generated reference as a key an operator cannot set.
func TestDefaultsRegistersNothingTheStructLacks(t *testing.T) {
	// Compared lower-cased on both sides. viper folds every key it
	// stores, so AllKeys reports "logging.file.maxbackups" for a
	// mapstructure tag of "maxBackups" - a comparison against the struct
	// spelling reports all 33 keys as orphaned and looks like a total
	// mismatch rather than a case difference.
	known := map[string]bool{}
	for _, p := range leafPaths() {
		known[strings.ToLower(p)] = true
	}

	var orphaned []string
	for _, k := range defaultsOnly().AllKeys() {
		if !known[strings.ToLower(k)] {
			orphaned = append(orphaned, k)
		}
	}
	sort.Strings(orphaned)
	if len(orphaned) > 0 {
		t.Errorf("%d defaults name keys the Config struct does not declare:\n  %s",
			len(orphaned), strings.Join(orphaned, "\n  "))
	}
}

// No test here asserts the environment binding, deliberately.
//
// An earlier draft added one, isolating bindEnvOverrides from
// AutomaticEnv, because with AutomaticEnv set the binding had no gate:
// deleting the call changed no observable behaviour and nothing went
// red. Dropping AutomaticEnv from Defaults removed the reason rather
// than the symptom. bindEnvOverrides is now the only thing that makes a
// key reachable, so TestEveryConfigKeyIsReachableFromTheEnvironment -
// which walks the same tree through Load - fails the moment it stops
// covering a leaf, and TestSecurityRelevantOverridesApply fails with it.
//
// Keeping the isolated test as well would have meant carrying a second
// assertion whose stated reason was no longer true. That is the kind of
// comment this ledger's standard is against.
