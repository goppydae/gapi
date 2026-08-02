package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func noopStart(*cobra.Command, []string) error { return nil }

// renderSurface captures what one version surface emits.
func renderSurface(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

// TestVersionSurfacesAgree closes GAPI-DIV-056's first half: the flag
// and the subcommand must render the same function, so they cannot
// drift. Before this, neither GAPI root set cobra's Version at all -
// `gapictl --version` exited 1 with "unknown flag".
func TestVersionSurfacesAgree(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func() *cobra.Command
	}{
		{"gapid", func() *cobra.Command { r, _, _ := NewGapidRoot(noopStart); return r }},
		{"gapictl", func() *cobra.Command { r, _ := NewGapictlRoot(); return r }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub := renderSurface(t, tc.make(), "version")
			flag := renderSurface(t, tc.make(), "--version")
			short := renderSurface(t, tc.make(), "-v")

			if sub != flag {
				t.Errorf("version subcommand and --version differ:\n sub=%q\nflag=%q", sub, flag)
			}
			if sub != short {
				t.Errorf("version subcommand and -v differ:\n sub=%q\n  -v=%q", sub, short)
			}
		})
	}
}

// TestVersionBlockNamesItsBinary closes the other half. Both binaries
// printed BYTE-IDENTICAL blocks that named neither, because
// SetBinaryNameAndVersion had zero callers and Summary() fell back to
// the literal "Runtime Core".
func TestVersionBlockNamesItsBinary(t *testing.T) {
	// Built in this order on purpose: the renderer holds one active
	// identity, so constructing gapictl after gapid must leave gapictl's
	// block naming gapictl. Reversing the assertion would pass on a
	// renderer that simply keeps whatever was registered last.
	gapid, _, _ := NewGapidRoot(noopStart)
	gapidBlock := renderSurface(t, gapid, "version")

	gapictl, _ := NewGapictlRoot()
	gapictlBlock := renderSurface(t, gapictl, "version")

	if !strings.HasPrefix(gapidBlock, "gapid:") {
		t.Errorf("gapid version first line = %q, want it to name gapid",
			strings.SplitN(gapidBlock, "\n", 2)[0])
	}
	if !strings.HasPrefix(gapictlBlock, "gapictl:") {
		t.Errorf("gapictl version first line = %q, want it to name gapictl",
			strings.SplitN(gapictlBlock, "\n", 2)[0])
	}
	if gapidBlock == gapictlBlock {
		t.Error("both binaries render the same block: neither can say which binary it is")
	}
}

// TestVersionBlockSharesOneColumnWidth pins the contract's layout rule.
// The block previously carried four different hardcoded paddings, so
// "Protobuf Schema Hash:" at 21 characters aligned with nothing.
func TestVersionBlockSharesOneColumnWidth(t *testing.T) {
	root, _, _ := NewGapidRoot(noopStart)
	block := renderSurface(t, root, "version")

	width := -1
	for _, line := range strings.Split(strings.TrimRight(block, "\n"), "\n") {
		colon := strings.Index(line, ":")
		if colon < 0 {
			t.Fatalf("version line has no label: %q", line)
		}
		rest := line[colon+1:]
		valueAt := colon + 1 + (len(rest) - len(strings.TrimLeft(rest, " ")))
		if width == -1 {
			width = valueAt
			continue
		}
		if valueAt != width {
			t.Errorf("value column starts at %d on %q, want %d for every row", valueAt, line, width)
		}
	}
}

// TestDaemonRootRefusesBareAndUnknown closes GAPI-DIV-057. gapid's root
// carried a RunE that WAS the daemon, so cobra handed any unmatched word
// to it and a mistyped subcommand booted a supervisor.
//
// Asserting RunE is nil is the load-bearing part: a guard that merely
// rejects positional arguments leaves the pass-through in place for the
// next person to depend on.
func TestDaemonRootRefusesBareAndUnknown(t *testing.T) {
	root, _, _ := NewGapidRoot(noopStart)

	if root.RunE != nil || root.Run != nil {
		t.Fatal("gapid root has a Run/RunE: unmatched words become positional arguments to the daemon")
	}

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := RunRoot(root, nil); !errors.Is(err, ErrNoCommand) {
		t.Errorf("bare invocation err = %v, want ErrNoCommand so the process exits non-zero", err)
	}

	root2, _, _ := NewGapidRoot(noopStart)
	root2.SetOut(&out)
	root2.SetErr(&out)
	err := RunRoot(root2, []string{"verison"})
	if err == nil {
		t.Fatal("gapid verison returned nil: a mistyped subcommand must fail, not run")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("gapid verison err = %q, want an unknown-command error", err)
	}
}

// TestStartActionRunsOnlyForStart proves the pass-through is gone by
// behaviour rather than by shape: the start action must fire for
// `start` and for nothing else.
func TestStartActionRunsOnlyForStart(t *testing.T) {
	var ran bool
	root, _, _ := NewGapidRoot(func(*cobra.Command, []string) error { ran = true; return nil })
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	_ = RunRoot(root, []string{"verison"})
	if ran {
		t.Fatal("the start action ran for a mistyped subcommand: this is the supervisor-boots-on-typo defect")
	}

	root2, _, _ := NewGapidRoot(func(*cobra.Command, []string) error { ran = true; return nil })
	root2.SetOut(&out)
	root2.SetErr(&out)
	if err := RunRoot(root2, []string{"start"}); err != nil {
		t.Fatalf("gapid start: %v", err)
	}
	if !ran {
		t.Error("the start action did not run for `start`")
	}
}

// flagSpec is the comparable shape of a flag: everything the contract
// says must match across peer binaries.
type flagSpec struct {
	name      string
	shorthand string
	defValue  string
	usage     string
}

func specsOf(fs *pflag.FlagSet) map[string]flagSpec {
	out := make(map[string]flagSpec)
	fs.VisitAll(func(f *pflag.Flag) {
		out[f.Name] = flagSpec{f.Name, f.Shorthand, f.DefValue, f.Usage}
	})
	return out
}

// TestRootsMatchTheSharedRegistrar closes GAPI-DIV-058. Shared
// definitions alone do not hold the contract - nothing stops a binary
// from adding a flag locally - so each root's PERSISTENT set is compared
// against a reference built by the registrar itself.
//
// A flag added to one root and not its peer, or with a different
// default or help text, fails here.
func TestRootsMatchTheSharedRegistrar(t *testing.T) {
	t.Run("daemon", func(t *testing.T) {
		ref := &cobra.Command{Use: "ref"}
		RegisterDaemonFlags(ref)
		want := specsOf(ref.PersistentFlags())

		root, _, _ := NewGapidRoot(noopStart)
		got := specsOf(root.PersistentFlags())
		// The version flag is part of the identity surface, not the
		// daemon vocabulary; the registrar does not define it.
		delete(got, "version")

		compareSpecs(t, "gapid", got, want)
	})

	t.Run("control", func(t *testing.T) {
		ref := &cobra.Command{Use: "ref"}
		RegisterControlFlags(ref)
		want := specsOf(ref.PersistentFlags())

		root, _ := NewGapictlRoot()
		got := specsOf(root.PersistentFlags())
		delete(got, "version")

		compareSpecs(t, "gapictl", got, want)
	})
}

func compareSpecs(t *testing.T, who string, got, want map[string]flagSpec) {
	t.Helper()
	for name, w := range want {
		g, ok := got[name]
		if !ok {
			t.Errorf("%s is missing persistent flag --%s defined by the registrar", who, name)
			continue
		}
		if g != w {
			t.Errorf("%s --%s = %+v, registrar defines %+v", who, name, g, w)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("%s defines persistent flag --%s locally; it belongs in the shared registrar", who, name)
		}
	}
}
