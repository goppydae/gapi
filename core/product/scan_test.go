package product_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// GAPI-DIV-061's last gate: no operator-facing string or path literal in
// the kernel spells the vendor's name.
//
// A one-time cleanup decays with the next log line, which is why this is
// a gate and not a commit. It scans STRING LITERALS through the AST
// rather than grepping lines, so comments, ledger ids and import paths
// are excluded structurally instead of by a pattern that has to keep
// guessing.
//
// The exclusions below are the WIRE class made explicit. Renaming any of
// them is a compatibility break, so the list exists to stop a later
// sweep from "finishing the job" - and, because each entry names why, to
// make an addition to it a decision someone has to write down.

// scanRoots are the packages whose output an operator can reach.
var scanRoots = []string{"core", "pkg/cli"}

// wireLiterals are exact string values that must keep the gapi spelling.
var wireLiterals = map[string]string{
	"gapi-quic": "core/transport: the QUIC ALPN. A protocol constant - " +
		"renaming it breaks every peer, and no operator reads it off a terminal.",
	"github.com/goppydae/gapi/adk/go/agent": "pkg/cli: the Go ADK's import " +
		"path, written into a generated main. A Go MODULE path, which is the " +
		"module's identity and not the product's - it stays the same string " +
		"whichever binary does the generating, and no operator reads it.",
}

// wirePrefixes are literal prefixes that must keep the gapi spelling.
var wirePrefixes = map[string]string{
	"gapi_": "core/metrics: metric names, referenced by dashboards, recording " +
		"rules and alerts outside this repo. Scraped identifiers, not prose.",
	"gapi.v1": "protobuf package names. Wire identifiers.",
	"gapi/v1": "protobuf file paths. Wire identifiers.",
}

// allowedIn is scoped by FILE, not by value, and that is deliberate.
//
// A global "gapid is fine" entry would have silently covered
// logattr.Module("gapid") - a structured-log field a goblind operator
// reads - alongside the panic message in gapid's own constructor. Two
// different things spelled identically. Naming the file makes each an
// individual decision, and a value that moves to another file surfaces
// again rather than staying quietly waived.
//
// The map can only shrink. A listed value that matches nothing fails
// below, so fixing a line forces its entry to be deleted.
var allowedIn = map[string][]string{
	// WIRE: the source field on a bus event, read by subscribers.
	"core/supervisor/supervisor.go":         {"gapid"},
	"core/supervisor/lifecycle_handlers.go": {"gapid"},
	"core/tui/actions.go":                   {"gapictl-tui"},

	// A binary naming ITSELF, which is what an identity surface is for,
	// plus the product each root declares.
	"pkg/cli/roots.go": {
		"gapi", "gapid",
		"gapid: start subcommand missing from daemon root: ",
	},
	"pkg/cli/gapictl.go": {"gapi", "gapictl"},

	// core/product's own diagnostics quote the names as examples.
	"core/product/product.go": {
		"like \"gapi\" or \"goblin\" - check the argument order at the call site",
		"path or cgroup name (GAPI-DIV-061).",
	},

	// DEBT. pkg/cli command examples name the binary they were written
	// for, and goblinctl mounts this tree under `agent` - so a goblinctl
	// operator reads gapictl's name in goblinctl's own help. Cobra
	// renders Example verbatim and the mount point is not known when the
	// command literal is built, so the fix is a restructure rather than a
	// rename. Tracked by GOBLIN-DIV-056.
	"pkg/cli/agent.go": {
		"Build Go agents from source and generate checksums.\n\nExamples:\n" +
			"  gapictl agent build agents/go/foundational/init/\n" +
			"  gapictl agent build --watch agents/go/coordination/cluster_join/\n" +
			"  gapictl agent build --sign --key=agent-signing.key agents/go/foundational/init/",
		"Create a new agent from template with proper structure.\n\nExamples:\n" +
			"  gapictl agent new my_service\n" +
			"  gapictl agent new --type=timer my_timer\n" +
			"  gapictl agent new --lang=python --type=service my_py_service",
	},
	"pkg/cli/agent_verify.go": {
		"Verify agent binary using hash chain and optional signature.\n\n" +
			"Verification steps:\n" +
			"  1. Binary hash (compares against .b3 file)\n" +
			"  2. Signature (if .sig file exists and --pubkey provided)\n" +
			"  3. Source hash (if --check-source and source available)\n\n" +
			"Examples:\n" +
			"  gapictl agent verify agents/my_service.go.service\n" +
			"  gapictl agent verify agents/my_service.go.service --pubkey=key.pub\n" +
			"  gapictl agent verify agents/my_service.go.service --check-source --source=src/agents/my_service.go.service",
	},
}

func TestNoVendorNameInOperatorFacingLiterals(t *testing.T) {
	root := repoRootFrom(t)

	var offenders []string
	seenAllowed := map[string]bool{}

	for _, sub := range scanRoots {
		walkGoFiles(t, root, filepath.Join(root, sub), func(rel string, f *ast.File, fset *token.FileSet) {
			// Import paths are BasicLits too; drop them wholesale.
			imports := map[*ast.BasicLit]bool{}
			for _, spec := range f.Imports {
				imports[spec.Path] = true
			}
			ast.Inspect(f, func(n ast.Node) bool {
				// Struct tags are BasicLits and are never operator-facing.
				if fld, ok := n.(*ast.Field); ok && fld.Tag != nil {
					imports[fld.Tag] = true
				}
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING || imports[lit] {
					return true
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil || !strings.Contains(strings.ToLower(val), "gapi") {
					return true
				}
				if allowed(rel, val) {
					seenAllowed[rel+"\x00"+val] = true
					return true
				}
				if _, ok := wireLiterals[val]; ok {
					return true
				}
				for p := range wirePrefixes {
					if strings.HasPrefix(val, p) {
						return true
					}
				}
				pos := fset.Position(lit.Pos())
				offenders = append(offenders,
					rel+":"+strconv.Itoa(pos.Line)+": "+strconv.Quote(val))
				return true
			})
		})
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("string literals in the kernel spell the vendor's name.\n"+
			"goblind links this code, and its operators have never heard of gapi "+
			"(GAPI-DIV-061). Name the ROLE instead - \"supervisor\" - or derive the "+
			"value from core/product. If the literal is a wire identifier, declare "+
			"it in wireLiterals or wirePrefixes with the reason:\n  %s",
			strings.Join(offenders, "\n  "))
	}

	// An entry that no longer matches anything is stale. Deleting it is
	// part of fixing the line it described.
	var stale []string
	for file, vals := range allowedIn {
		for _, val := range vals {
			if !seenAllowed[file+"\x00"+val] {
				stale = append(stale, file+": "+strconv.Quote(val))
			}
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("allowedIn entries match nothing in the tree - the literals were "+
			"fixed or moved, so delete the declarations:\n  %s", strings.Join(stale, "\n  "))
	}
}

// allowed reports whether file is permitted to contain val.
func allowed(file, val string) bool {
	for _, v := range allowedIn[file] {
		if v == val {
			return true
		}
	}
	return false
}

// repoRootFrom walks up to the module root.
func repoRootFrom(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's working directory")
		}
		dir = parent
	}
}

// walkGoFiles parses every non-test, non-generated .go file under dir.
func walkGoFiles(t *testing.T, root, dir string, visit func(rel string, f *ast.File, fset *token.FileSet)) {
	t.Helper()
	fset := token.NewFileSet()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") ||
			strings.HasSuffix(path, ".pb.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return perr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		visit(rel, f, fset)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
}
