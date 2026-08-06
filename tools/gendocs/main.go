// Copyright (c) 2025 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

// Command gendocs renders gapi's reference documentation.
//
// It lives here rather than in pkg/docsgen because it needs the real
// command roots, which are per-repo: goblin has its own copy calling its
// own constructors against the same renderers. magelib shells out to
// `go run ./tools/gendocs` and never learns what a cobra command is.
//
// Usage: gendocs [output-root]
//
// The single optional argument is the directory to write beneath, and it
// is magelib's contract with every generator. Under an ordinary
// generation it is "." and output lands in the working tree; under
// `mage docs:check` it is a temporary directory, so the drift gate can
// regenerate and byte-compare without repairing the drift it is
// measuring.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/goppydae/gapi/core/config"
	"github.com/goppydae/gapi/core/product"
	"github.com/goppydae/gapi/internal/safeio"
	"github.com/goppydae/gapi/pkg/cli"
	"github.com/goppydae/gapi/pkg/docsgen"
)

const productName = "gapi"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
}

func run() error {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	// Declared before anything reads a product-scoped default. This is
	// the point gendocs's process starts, which is where core/product
	// says identity is declared; DefaultControlAddr and friends panic on
	// an unset identity rather than guessing another product's port.
	product.Set(productName)

	version, err := repoVersion()
	if err != nil {
		return err
	}

	// gapictl's POPULATED root, not NewGapictlRoot().
	//
	// The constructor returns a fresh tree carrying only the persistent
	// flags and `version` - measured at 2 commands against the
	// singleton's 25 - because five func init() blocks register the verbs
	// against the package-level root instead. Generating from the
	// constructor would publish a reference with ping, shutdown, tui and
	// every agent, crypto and lifecycle verb missing, and the drift gate
	// would hold that stub steady forever, since it would be comparing
	// the stub to itself.
	ctl := cli.GetRoot()

	// gapid's root IS built by its constructor, which takes the start
	// action as a parameter so pkg/cli never imports core/supervisor.
	//
	// THE ACTION MUST BE NON-NIL EVEN THOUGH IT IS NEVER RUN. Passing nil
	// leaves `start` with a nil RunE, so cobra reports it as not runnable,
	// IsAvailableCommand() is false, and the generator SILENTLY OMITS IT -
	// measured: the first run of this tool produced gapid.md and
	// gapid_version.md and no gapid_start.md, dropping the daemon's
	// central verb and all nine of its flags from both the reference and
	// the man pages. Nothing errors; the page set is simply smaller, and
	// the drift gate would then hold that omission steady forever.
	//
	// The same rule reaches further than this one call: any group command
	// whose children all lose their Run disappears from the reference
	// too.
	neverRun := func(*cobra.Command, []string) error {
		return fmt.Errorf("gendocs: the documented start action is not executable")
	}
	daemon, _, _ := cli.NewGapidRoot(neverRun)

	// A slice rather than a map: the order is fixed by the literal, so
	// nothing here depends on map iteration order. Both pages are
	// byte-compared by the drift gate, and an unstable walk is the
	// cheapest way to make a gate flap for no reason.
	for _, b := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{"gapictl", ctl},
		{"gapid", daemon},
	} {
		cliDir := filepath.Join(root, "docs", "content", "reference", "cli", b.name)
		if err := docsgen.CLI(b.cmd, cliDir); err != nil {
			return err
		}
		if err := docsgen.Man(b.cmd, filepath.Join(root, "docs", "man", "man1"), version); err != nil {
			return err
		}
	}

	if err := renderConfig(root, version); err != nil {
		return err
	}

	// Section 7 is the one written page, converted rather than
	// generated. It is NOT skipped when missing: goblin's current
	// Docs:Man names a source file that does not exist and skips it in
	// silence, which is how a man page target can be green for months
	// while producing nothing.
	overview := filepath.Join("docs", "content", "user", "overview.md")
	if err := docsgen.Overview(overview,
		filepath.Join(root, "docs", "man", "man7", productName+".7"),
		productName, version); err != nil {
		return err
	}
	return nil
}

// renderConfig builds the schema model once and renders its three forms.
func renderConfig(root, version string) error {
	model, err := docsgen.BuildConfigModel(docsgen.ConfigOptions{
		Product:   productName,
		Schema:    &config.Config{},
		Defaults:  config.Defaults(),
		SourceDir: filepath.Join("core", "config"),
		EnvKeyFor: config.EnvKeyFor,
	})
	if err != nil {
		return err
	}

	page := filepath.Join(root, "docs", "content", "reference", "configuration.md")
	if err := write(page, docsgen.ConfigMarkdown(model, 20)); err != nil {
		return err
	}

	manPage := filepath.Join(root, "docs", "man", "man5", productName+".conf.5")
	if err := write(manPage, docsgen.ConfigMan(model, version)); err != nil {
		return err
	}

	data, err := docsgen.DefaultsJSON(model, "core/config.setDefaults")
	if err != nil {
		return err
	}
	return write(filepath.Join(root, "docs", "data", "defaults.json"), data)
}

// repoVersion reads the VERSION file and nothing else.
//
// Deliberately NOT magelib.Version(), which prefers RELEASE_VERSION and
// the tag ref before falling back to the file. Those make the output a
// function of the ENVIRONMENT, so the same tree would generate different
// man page headers on a tag build than on a branch build - and the drift
// gate, which compares committed bytes against a fresh render, would go
// red on precisely the release commit it most needs to be green on.
//
// Reading the file makes the artifact a function of the tree. The
// consequence is intended: bumping VERSION regenerates the man pages, so
// a page names the release it documents.
func repoVersion() (string, error) {
	// The VERSION file is read from the WORKING TREE, not from the output
	// root: the output root is a scratch directory under the drift gate
	// and has no VERSION in it.
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return "", fmt.Errorf("reading VERSION: %w", err)
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "", fmt.Errorf("VERSION is empty; man page headers would carry no version")
	}
	return v, nil
}

// write routes through safeio, the audited chokepoint gapi's lint
// carve-out names: the output root is a command-line argument, so every
// path built from it is variable by contract and is cleaned and made
// absolute in one place.
//
// safeio does NOT silence G703 the way it silences G304 - measured, the
// finding survives the chokepoint, because gosec's taint analysis
// follows the value through. gendocs is the first code in this repo to
// read os.Args directly and write to a path derived from it; cobra flag
// values are not taint sources, which is why nothing else here trips the
// rule. See pkg/docsgen.writeFile for why the suppression is inline and
// narrow rather than a repo-wide exclude.
func write(path string, data []byte) error {
	p, err := safeio.Resolve(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil { // #nosec G703 -- see write's comment
		return fmt.Errorf("creating %s: %w", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil { // #nosec G703 -- see write's comment
		return fmt.Errorf("writing %s: %w", p, err)
	}
	return nil
}
