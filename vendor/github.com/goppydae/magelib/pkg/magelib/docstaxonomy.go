// Copyright (c) 2026 Steven Verhelle (enqack)
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.
//
// SPDX-License-Identifier: MPL-2.0

package magelib

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// hrefAny matches an href in either quoted or minified-unquoted form.
//
// Hugo renders with --minify, which emits `href=/tags/` rather than
// `href="/tags/"`. A pattern written against the quoted form matches
// nothing on a minified site, which has already produced two wrong
// readings of these sites in one day.
var hrefAny = regexp.MustCompile(`href=(?:"([^"]+)"|([^\s>]+))`)

// CheckAdvertisedTaxonomies asserts that every link the site advertises
// resolves, and that an advertised TAXONOMY index carries at least one
// term.
//
// THE DEFECT IS A SIDEBAR THAT PROMISES SOMETHING THE SITE DOES NOT HAVE
// (MAGELIB-DIV-013). The shared Hugo base declares `category` and `tag`
// taxonomies and hardcodes a menu naming both, and DocsSync materialises
// that into every consumer - so all three published sites carried a
// Categories and a Tags link to indexes that nothing populates. Measured
// when the entry was filed: zero pages in any repo declare either.
//
// IT IS DIRECTIONLESS ON PURPOSE, which the entry insists on because the
// underlying question is undecided. It closes if the taxonomies are
// POPULATED, because the terms then exist. It closes if they are
// DROPPED, because there is then no advertised link left to resolve. And
// the second half is what stops the fix being applied halfway: a menu
// entry whose target the site does not render is a failure too, so
// removing the taxonomies while leaving the menu is caught rather than
// passing vacuously - which is this defect one direction later.
//
// It reads the EFFECTIVE Hugo configuration rather than the template,
// because the template is not what the site was built from: a repo may
// add its own config.yaml, and Hugo REPLACES rather than merges a config
// slice. Asking hugo what it actually used is the only source that
// cannot be stale.
func CheckAdvertisedTaxonomies(cfg DocsConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	conf, err := effectiveHugoConfig(cfg)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "magelib-taxonomy-")
	if err != nil {
		return fmt.Errorf("taxonomy check: temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	if err := DocsSync(cfg); err != nil {
		return fmt.Errorf("taxonomy check: sync assets: %w", err)
	}
	if err := runStreamed("hugo", append(hugoArgs(cfg), "--minify", "--destination", tmp)...); err != nil {
		return fmt.Errorf("taxonomy check: render: %w", err)
	}

	links := conf.advertisedLinks()

	// THE GATE HAS TWO WORLDS AND IS NOT VACUOUS IN EITHER, which is what
	// "directionless" has to mean in practice.
	//
	// With NO taxonomies declared - the dropped resolution - there is no
	// advertised link to resolve, and the thing worth asserting is that
	// the drop TOOK EFFECT. Hugo's built-in defaults are category and
	// tag, so deleting the declaration restores them rather than removing
	// them; only an explicit empty map removes them. This catches that
	// exact reversal, which is a trap this codebase walked into while
	// making the change.
	if len(conf.Taxonomies) == 0 {
		return assertNoTaxonomyPages(tmp, links)
	}

	// With taxonomies declared, something must advertise them or nothing
	// checks them - and a walker that silently matches nothing reports
	// success for the rest of its life.
	if len(links) == 0 {
		return fmt.Errorf(
			"the configuration declares taxonomies %v and NO menu advertises any: either "+
				"advertise them or drop the declaration, because as it stands nothing links to "+
				"pages the site renders", conf.Taxonomies)
	}

	plural := map[string]bool{}
	for _, p := range conf.Taxonomies {
		plural[p] = true
	}

	var problems []string
	for _, l := range links {
		rendered := filepath.Join(tmp, filepath.FromSlash(strings.Trim(l.URL, "/")), "index.html")
		body, rerr := os.ReadFile(rendered) // #nosec G304 -- path derived from config under a temp root
		if rerr != nil {
			problems = append(problems, fmt.Sprintf(
				"  %q advertises %s and the site renders no such page", l.Name, l.URL))
			continue
		}
		seg := strings.Trim(l.URL, "/")
		if !plural[seg] {
			continue // an ordinary link; rendering was the whole claim
		}
		if n := countTermLinks(body, seg); n == 0 {
			problems = append(problems, fmt.Sprintf(
				"  %q advertises the %s taxonomy and its index lists no terms", l.Name, seg))
		}
	}

	if len(problems) == 0 {
		fmt.Printf("taxonomy check: %d advertised links, all resolving\n", len(links))
		return nil
	}
	sort.Strings(problems)
	// The remedy goes in the HEADER rather than after the list: an error
	// string may not end in punctuation (ST1005), and advice trailing a
	// list is read last anyway.
	return fmt.Errorf(
		"advertised taxonomy check failed - either populate these taxonomies or stop "+
			"advertising them, and MAGELIB-DIV-013 closes either way:\n%s",
		strings.Join(problems, "\n"))
}

// countTermLinks counts distinct term pages an index links to, e.g.
// /tags/<term>/ from /tags/.
//
// A TERM IS A DIRECTORY, AN OUTPUT FORMAT IS A FILE, AND THE FIRST
// VERSION COUNTED BOTH. Hugo renders /categories/index.xml and
// /categories/index.print.html beside the index, so a check that took
// any path under the taxonomy as a term found two of them on an index
// with no terms at all - and reported both taxonomies populated, which
// is the vacuous pass this gate exists to prevent. It survived only
// because MAGELIB-DIV-013 requires the check be demonstrated FAILING
// before the fix lands, and it did not fail.
//
// So a term link must end in "/" - hugo emits term pages as directories
// - and carry no extension in its final segment.
func countTermLinks(body []byte, seg string) int {
	prefix := "/" + seg + "/"
	terms := map[string]bool{}
	for _, m := range hrefAny.FindAllSubmatch(body, -1) {
		href := string(m[1])
		if href == "" {
			href = string(m[2])
		}
		p := strings.SplitN(href, "?", 2)[0]
		if !strings.HasPrefix(p, prefix) || !strings.HasSuffix(p, "/") {
			continue
		}
		rest := strings.Trim(strings.TrimPrefix(p, prefix), "/")
		// One level below the index, and not a file: deeper paths belong
		// to a term rather than naming one.
		if rest == "" || strings.Contains(rest, "/") || strings.Contains(rest, ".") {
			continue
		}
		terms[rest] = true
	}
	return len(terms)
}

// hugoConfig is the subset of the effective configuration this gate
// reads.
type hugoConfig struct {
	Taxonomies map[string]string `json:"taxonomies"`
	Menus      map[string][]struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"menus"`
}

type advertised struct{ Name, URL string }

// advertisedLinks returns every INTERNAL menu link the configuration
// declares. External links are excluded deliberately: this gate asserts
// the site renders what it advertises, and it cannot render github.com.
func (c hugoConfig) advertisedLinks() []advertised {
	var out []advertised
	for _, entries := range c.Menus {
		for _, e := range entries {
			if e.URL == "" || !strings.HasPrefix(e.URL, "/") {
				continue
			}
			out = append(out, advertised{Name: e.Name, URL: path.Clean(e.URL)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URL < out[j].URL })
	return out
}

// effectiveHugoConfig asks hugo what configuration it actually resolves,
// rather than reading the template magelib rendered.
func effectiveHugoConfig(cfg DocsConfig) (hugoConfig, error) {
	args := append(hugoArgs(cfg), "config", "--format", "json")
	out, err := exec.Command("hugo", args...).Output() // #nosec G204 -- args built from validated DocsConfig
	if err != nil {
		return hugoConfig{}, fmt.Errorf("taxonomy check: reading hugo config: %w", err)
	}
	// The devshell hook prints a banner before hugo's own output.
	i := strings.Index(string(out), "{")
	if i < 0 {
		return hugoConfig{}, fmt.Errorf("taxonomy check: hugo config produced no JSON object")
	}
	var conf hugoConfig
	if err := json.Unmarshal(out[i:], &conf); err != nil {
		return hugoConfig{}, fmt.Errorf("taxonomy check: parsing hugo config: %w", err)
	}
	return conf, nil
}

// assertNoTaxonomyPages is the check for a site that declares no
// taxonomies: nothing may render under Hugo's default taxonomy paths,
// and no menu may advertise one.
//
// It exists because "taxonomies: {}" and "no taxonomies key" are
// different configurations. The first removes them; the second restores
// Hugo's category and tag defaults, so a well-meant deletion of the
// empty map silently brings back the empty indexes MAGELIB-DIV-013 was
// filed about.
func assertNoTaxonomyPages(root string, links []advertised) error {
	var problems []string
	for _, seg := range []string{"categories", "tags"} {
		if _, err := os.Stat(filepath.Join(root, seg, "index.html")); err == nil {
			problems = append(problems, fmt.Sprintf(
				"  the site declares no taxonomies and still renders /%s/ - Hugo's defaults are "+
					"back, which an absent `taxonomies` key does rather than an empty one", seg))
		}
		for _, l := range links {
			if strings.Trim(l.URL, "/") == seg {
				problems = append(problems, fmt.Sprintf(
					"  %q advertises /%s/ and the site declares no taxonomies", l.Name, seg))
			}
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("advertised taxonomy check failed:\n%s", strings.Join(problems, "\n"))
	}
	fmt.Println("taxonomy check: no taxonomies declared, and none rendered or advertised")
	return nil
}
