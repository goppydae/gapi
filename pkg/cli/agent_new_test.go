package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// scaffoldAt runs 'agent new' for one (lang, type) pair into dir and
// returns the path it wrote. The command reads its inputs from package
// globals, so they are set and restored here rather than in each test.
func scaffoldAt(t *testing.T, dir, lang, typ, name string) (string, error) {
	t.Helper()

	oldLang, oldType, oldOut := agentLang, agentType, agentOutput
	t.Cleanup(func() { agentLang, agentType, agentOutput = oldLang, oldType, oldOut })
	agentLang, agentType, agentOutput = lang, typ, dir

	if err := runAgentNew(&cobra.Command{}, []string{name}); err != nil {
		return "", err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read scaffold dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly one scaffolded file in %s, got %d", dir, len(entries))
	}
	return filepath.Join(dir, entries[0].Name()), nil
}

// declaredType extracts the agent type the scaffolded SOURCE declares.
//
// Reading the file rather than trusting the table is the whole point:
// the defect this test exists for (GAPI-DIV-054) was a correct-looking
// selection that rendered another type's template, so an assertion that
// only checked which template the table names would have passed against
// the broken code.
func declaredType(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scaffold: %v", err)
	}

	// Python declares TYPE = "service"; Go declares "type": "service"
	// inside its describe payload.
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?m)^TYPE\s*=\s*"([a-z]+)"`),
		regexp.MustCompile(`"type"\s*:\s*"([a-z]+)"`),
	} {
		if m := re.FindSubmatch(body); m != nil {
			return string(m[1])
		}
	}
	t.Fatalf("no type declaration found in %s", path)
	return ""
}

// TestAgentNew_EveryAdvertisedPairScaffoldsItsOwnType walks the CARTESIAN
// PRODUCT of the scaffold matrix's axes rather than a hand-written list,
// so a language or type added to one axis is immediately demanded of
// every other - the failure mode that produced GAPI-DIV-054 was a type
// the validator advertised and no template served.
//
// Deleting any single cell from agentScaffolds fails this test: the axes
// are the UNION over the map, so removing {python, socket} leaves
// "socket" on the type axis (Go still has it) and the pair then has no
// scaffold.
func TestAgentNew_EveryAdvertisedPairScaffoldsItsOwnType(t *testing.T) {
	langs, types := scaffoldLangs(), scaffoldTypes()
	if len(langs) == 0 || len(types) == 0 {
		t.Fatalf("empty scaffold matrix: langs=%v types=%v", langs, types)
	}

	for _, lang := range langs {
		for _, typ := range types {
			t.Run(lang+"/"+typ, func(t *testing.T) {
				path, err := scaffoldAt(t, t.TempDir(), lang, typ, "probe_agent")
				if err != nil {
					t.Fatalf("agent new --lang %s --type %s: %v", lang, typ, err)
				}

				if got := declaredType(t, path); got != typ {
					t.Errorf("scaffold declares type %q, asked for %q (file %s)",
						got, typ, filepath.Base(path))
				}

				// Python's file name is what discovery routes on, so the
				// suffix is behaviour and not decoration. Go builds a
				// package, so its type lives only in the source.
				if lang == "python" {
					want := fmt.Sprintf(".py.%s", typ)
					if !strings.HasSuffix(path, want) {
						t.Errorf("scaffold named %q, want suffix %q",
							filepath.Base(path), want)
					}
					assertPythonParses(t, path)
				}
			})
		}
	}
}

// assertPythonParses compiles the scaffolded module.
//
// Without this the suite could not tell a working template from one with
// a syntax error: declaredType reads TYPE with a regexp, which matches
// perfectly well in a file Python refuses to import. The templates are
// the only place in this package where non-Go source is authored, so
// nothing else would catch it before an operator did.
//
// The dev shell provides python3 (flake.nix), so the skip below does not
// fire in CI. It exists for a lone 'go test ./...' outside the shell, and
// it names what went unchecked rather than passing quietly.
func assertPythonParses(t *testing.T, path string) {
	t.Helper()

	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skipf("python3 not on PATH: scaffold %s NOT syntax-checked", filepath.Base(path))
	}

	out, err := exec.Command(py, "-m", "py_compile", path).CombinedOutput()
	if err != nil {
		t.Errorf("scaffolded %s does not compile: %v\n%s", filepath.Base(path), err, out)
	}
}

// TestAgentNew_EveryScaffoldTemplateExists catches a matrix entry that
// names a template no longer embedded. Without it, a renamed .tmpl would
// surface only when someone scaffolded that exact pair.
func TestAgentNew_EveryScaffoldTemplateExists(t *testing.T) {
	for key, s := range agentScaffolds {
		if _, err := templatesFS.ReadFile(s.template); err != nil {
			t.Errorf("%s/%s names %s, which is not embedded: %v",
				key.lang, key.typ, s.template, err)
		}
	}
}

// TestAgentNew_UnsupportedPairErrorsAndWritesNothing pins the second half
// of the exit: the lookup must never fall back to a different type, and a
// rejected pair must leave no file behind. Exit status 0 with a wrong
// file on disk is what made the original defect indistinguishable from
// success.
func TestAgentNew_UnsupportedPairErrorsAndWritesNothing(t *testing.T) {
	cases := []struct{ lang, typ, wantIn string }{
		{"ruby", "service", "unsupported language"},
		{"python", "daemon", "unsupported type"},
	}

	for _, tc := range cases {
		t.Run(tc.lang+"/"+tc.typ, func(t *testing.T) {
			dir := t.TempDir()

			oldLang, oldType, oldOut := agentLang, agentType, agentOutput
			t.Cleanup(func() { agentLang, agentType, agentOutput = oldLang, oldType, oldOut })
			agentLang, agentType, agentOutput = tc.lang, tc.typ, dir

			err := runAgentNew(&cobra.Command{}, []string{"probe_agent"})
			if err == nil {
				t.Fatalf("agent new --lang %s --type %s succeeded, want an error", tc.lang, tc.typ)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", err, tc.wantIn)
			}

			entries, rerr := os.ReadDir(dir)
			if rerr != nil {
				t.Fatalf("read dir: %v", rerr)
			}
			if len(entries) != 0 {
				t.Errorf("rejected pair still wrote %d file(s)", len(entries))
			}
		})
	}
}

// TestAgentNew_FlagHelpMatchesTheMatrix keeps the advertised surface and
// the servable surface from drifting apart. The flag help is what an
// operator reads before choosing a type, so a literal there that the
// matrix cannot serve is the same broken promise in a different place.
func TestAgentNew_FlagHelpMatchesTheMatrix(t *testing.T) {
	for flag, want := range map[string][]string{
		"lang": scaffoldLangs(),
		"type": scaffoldTypes(),
	} {
		f := agentNewCmd.Flags().Lookup(flag)
		if f == nil {
			t.Fatalf("agent new has no --%s flag", flag)
		}
		for _, v := range want {
			if !strings.Contains(f.Usage, v) {
				t.Errorf("--%s help %q omits %q", flag, f.Usage, v)
			}
		}
	}
}
