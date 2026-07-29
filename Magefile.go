//go:build mage
// +build mage

package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/goppydae/magelib/pkg/magelib"
	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
	"github.com/zeebo/blake3"
)

// versionLdflags stamps the resolved version into core/version, the shared
// injection point read by both binaries (VERSION file is the source of truth).
func versionLdflags() string {
	return "-X github.com/goppydae/gapi/core/version.GAPIVersion=" + magelib.Version()
}

func Build() error {
	mg.Deps(checkHermetic)
	fmt.Printf("Building gapid and gapictl (version %s)...\n", magelib.Version())

	if err := sh.Run("go", "build", "-ldflags", versionLdflags(), "-o", "bin/gapid", "./cmd/gapid"); err != nil {
		return err
	}

	if err := sh.Run("go", "build", "-ldflags", versionLdflags(), "-o", "bin/gapictl", "./cmd/gapictl"); err != nil {
		return err
	}

	// Calculate and write BLAKE3 hashes
	for _, bin := range []string{"bin/gapid", "bin/gapictl"} {
		if err := hashFile(bin); err != nil {
			return fmt.Errorf("hashing %s failed: %w", bin, err)
		}
	}

	fmt.Println("Build complete: bin/gapid, bin/gapictl (with .b3 checksums)")
	return nil
}

func hashFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	h := blake3.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}

	sum := h.Sum(nil)
	hexSum := hex.EncodeToString(sum)

	return os.WriteFile(path+".b3", []byte(hexSum+"\n"), 0644)
}

func Install() error {
	mg.Deps(checkHermetic)
	fmt.Println("Installing gapid and gapictl...")

	if err := sh.Run("go", "install", "-ldflags", versionLdflags(), "./cmd/gapid"); err != nil {
		return err
	}

	if err := sh.Run("go", "install", "-ldflags", versionLdflags(), "./cmd/gapictl"); err != nil {
		return err
	}

	fmt.Println("Installed to $GOPATH/bin")
	return nil
}

// Test runs all tests with the race detector (the suite CI runs)
func Test() error {
	mg.Deps(checkHermetic)
	fmt.Println("Running tests (-race)...")
	return sh.RunV("go", "test", "-race", "./...")
}

// TestShort runs the fast inner-loop subset
func TestShort() error {
	mg.Deps(checkHermetic)
	fmt.Println("Running short tests...")
	return sh.RunV("go", "test", "-short", "./...")
}

// Fuzz runs every Fuzz* target for a bounded fuzztime
func Fuzz() error {
	mg.Deps(checkHermetic)
	return magelib.FuzzAll("10s")
}

// Vuln runs govulncheck (named offline exception: skipped loudly when offline)
func Vuln() error {
	mg.Deps(checkHermetic)
	return magelib.Vuln()
}

// Doctor validates the local environment against the ecosystem pins
func Doctor() error {
	return magelib.Doctor(magelib.DoctorConfig{
		ReplaceTargets: []string{"../magelib"},
		ProtoPlugins:   []string{"buf", "protoc-gen-go", "protoc-gen-go-grpc"},
		GopyVersion:    "v0.4.10",
		RequiredEnv:    []string{"GOBIN"},
		SharedTools:    []string{"buf", "golangci-lint", "gosec", "govulncheck", "mage", "goimports", "mkdocs", "pandoc"},
	})
}

// EnvCheck compares the sibling dev shells' tool inventories; skew is red
func EnvCheck() error {
	return magelib.CheckShellUnification(
		map[string]string{"gapi": ".", "goblin": "../goblin", "magelib": "path:../magelib"},
		[]string{"go", "gcc", "protoc", "buf", "golangci-lint", "gosec", "govulncheck", "mage", "goimports"},
	)
}

// TestUnit runs only unit tests
func TestUnit() error {
	mg.Deps(checkHermetic)
	fmt.Println("Running unit tests...")
	return sh.RunV("go", "test", "-v", "./internal/...")
}

// TestADK runs ADK integration tests
func TestADK() error {
	mg.Deps(Build, Python{}.Build)
	fmt.Println("Running ADK integration tests...")
	return sh.RunV("go", "test", "-v", "./test/adk/...")
}

// TestPid1 runs the PID-1 container e2e: gapid as init of a rootless
// podman container (orphan reaping, signal semantics, teardown order).
func TestPid1() error {
	mg.Deps(checkHermetic)
	fmt.Println("Running PID-1 container e2e (rootless podman)...")
	return sh.RunV("go", "test", "-tags", "pid1", "-timeout", "10m", "-v", "-run", "TestPid1", "./test/pid1/")
}

// TestE2E runs end-to-end tests
func TestE2E() error {
	mg.Deps(Build, Python{}.Build)
	fmt.Println("Running E2E tests...")
	return sh.RunV("./test/e2e.sh")
}

// TestIntegrity runs integrity verification tests
func TestIntegrity() error {
	mg.Deps(Build)
	fmt.Println("Running integrity tests...")
	return sh.RunV("./test/test_integrity.sh")
}

// TestTimer runs timer agent tests
func TestTimer() error {
	mg.Deps(Build, Python{}.Build)
	fmt.Println("Running timer tests...")
	return sh.RunV("./test/test_timer.sh")
}

// Release cross-compiles pure-Go release artifacts (CGO_ENABLED=0) for
// linux/{amd64,arm64} and darwin/{amd64,arm64} into dist/, with a SHA256SUMS
// manifest. Signing (minisign) and the signed tag are operator-gated and
// printed as a stub.
func Release() error {
	mg.Deps(checkHermetic)
	return magelib.Release("gapi", "dist", versionLdflags(), map[string]string{
		"gapid":   "./cmd/gapid",
		"gapictl": "./cmd/gapictl",
	})
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning build artifacts...")

	dirs := []string{"bin", "build", "dist"}
	for _, dir := range dirs {
		if err := sh.Rm(dir); err != nil {
			fmt.Printf("Warning: failed to remove %s: %v\n", dir, err)
		}
	}

	fmt.Println("Clean complete")
	return nil
}

// Proto generates protobuf code through the buf gate (generate, lint,
// breaking against HEAD). Generated code lands directly in pkg/proto
// (buf.gen.yaml module opt strips the module prefix from go_package).
func Proto() error {
	mg.Deps(checkHermetic)
	if err := magelib.BufGenerate(".git#ref=HEAD"); err != nil {
		return err
	}
	fmt.Println("Protobuf generation complete (buf generate + lint + breaking)")
	return nil
}

// Fmt formats all Go code
func Fmt() error {
	return magelib.Fmt()
}

// Lint runs linters.
//
// Rule-level gosec carve-outs (GAPI-DIV-024):
//   - G204: gapid is a process supervisor; launching operator-registered
//     agent binaries and interpreters with discovered paths is the product.
//     Discovery roots are fenced (RUNTIME_AGENT_PATH) and binaries are
//     signature-verified before start (agentreg integrity check).
//   - G304: every variable-path open routes through internal/safeio, the
//     audited chokepoint (clean + absolute, root-confined where a root
//     exists); the rule now only fires inside that package.
func Lint() error {
	mg.Deps(checkHermetic)
	return magelib.Lint("G204", "G304")
}

// Tidy runs go mod tidy
func Tidy() error {
	return magelib.Tidy()
}

// Dev runs the development build and starts gapid
func Dev() error {
	mg.Deps(Build, Python{}.Build)
	fmt.Println("Starting gapid in development mode...")
	return sh.RunV("./bin/gapid")
}

// All runs fmt, tidy, build, and test
func All() error {
	mg.Deps(Fmt, Tidy, Build, Test)
	fmt.Println("All tasks complete")
	return nil
}

// Documentation tasks
type Docs mg.Namespace

// Html generates the static documentation site using MkDocs
func (Docs) Html() error {
	fmt.Println("Generating HTML documentation...")
	// Check for mkdocs
	if _, err := exec.LookPath("mkdocs"); err != nil {
		return fmt.Errorf("mkdocs not found. Run 'nix develop' to get documentation tools")
	}
	return sh.RunV("mkdocs", "build")
}

// Man generates man pages from markdown files using Pandoc
func (Docs) Man() error {
	fmt.Println("Generating Man pages...")
	// Check for pandoc
	if _, err := exec.LookPath("pandoc"); err != nil {
		return fmt.Errorf("pandoc not found. Run 'nix develop' to get documentation tools")
	}

	if err := os.MkdirAll("man/man1", 0755); err != nil {
		return err
	}

	// Generate main man page
	// Metadata in frontmatter (title, section) is respected by pandoc if present,
	// otherwise we might want to set title via flags.
	// We'll generate a basic man page for the main entry points.

	pages := map[string]string{
		"docs/index.md":           "man/man1/gapi.1",
		"docs/library_usage.md":   "man/man1/gapi-library.1",
		"docs/getting-started.md": "man/man1/gapi-quickstart.1",
	}

	for src, dst := range pages {
		fmt.Printf("Generating %s -> %s\n", src, dst)
		if err := sh.Run("pandoc", src, "-s", "-t", "man", "-o", dst); err != nil {
			return fmt.Errorf("failed to generate %s: %w", dst, err)
		}
	}

	fmt.Println("Man pages generated in ./man/man1")
	return nil
}

// checkHermetic ensures tools are running from Nix store
func checkHermetic() error {
	return magelib.CheckHermetic()
}

// Python namespace for python-related tasks
type Python mg.Namespace

// Build builds the Python bindings (replaces Makefile)
func (Python) Build() error {
	mg.Deps(checkHermetic)
	fmt.Println("Building Python ADK bindings...")

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	nativeDir := filepath.Join(cwd, "adk/python/gapi/native")

	// Create native directory if it doesn't exist (though gopy gen should verify path)
	if err := os.MkdirAll(nativeDir, 0755); err != nil {
		return err
	}

	// 1. gopy gen
	// gopy gen -no-make -vm=python3 -output=adk/python/gapi/native github.com/goppydae/gapi/adk/go
	fmt.Println("Running gopy gen...")
	if err := sh.Run("gopy", "gen", "-no-make", "-vm=python3", "-output=adk/python/gapi/native", "github.com/goppydae/gapi/adk/go"); err != nil {
		return fmt.Errorf("gopy gen failed: %w", err)
	}

	// Change to native directory for subsequent commands to match Makefile behavior
	// or use absolute paths. Makefile runs inside the dir. Let's act on files directly.

	// 2. goimports
	fmt.Println("Running goimports...")
	if err := sh.Run("goimports", "-w", filepath.Join(nativeDir, "adk.go")); err != nil {
		return fmt.Errorf("goimports failed: %w", err)
	}

	// 3. go build -buildmode=c-shared
	fmt.Println("Building adk_go.so...")
	// Note: We build inside the native dir to keep files together or specify output
	// Makefile: $(GOBUILD) -buildmode=c-shared -o adk_go$(LIBEXT) adk.go
	// We need to run this command *in* the native directory because it compiles the generated adk.go
	// which depends on imports.

	// No -mod flag: it is illegal in workspace mode (go.work), so hardcoding
	// -mod=mod made this target fail in the dev shell and pass in CI only
	// because CI sets GOWORK=off globally. The default (readonly) is correct
	// in both modes; go.work resolves the sibling modules in the dev shell.
	buildCmd := exec.Command("go", "build", "-buildmode=c-shared", "-o", "adk_go.so", "adk.go")
	buildCmd.Dir = nativeDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	// 4. python build.py
	fmt.Println("Running python build.py...")
	pyCmd := exec.Command("python3", "build.py")
	pyCmd.Dir = nativeDir
	pyCmd.Stdout = os.Stdout
	pyCmd.Stderr = os.Stderr
	if err := pyCmd.Run(); err != nil {
		// Try fallback to known nix path if "python3" generic fails (unlikely in nix shell but safe)
		return fmt.Errorf("python build.py failed: %w", err)
	}

	// 5. gcc link
	fmt.Println("Linking _adk.so...")
	// Gather python flags
	cflags, err := sh.Output("python3-config", "--cflags")
	if err != nil {
		return fmt.Errorf("failed to get python cflags: %w", err)
	}
	ldflags, err := sh.Output("python3-config", "--ldflags", "--embed")
	if err != nil {
		return fmt.Errorf("failed to get python ldflags: %w", err)
	}

	// Manually construct the gcc command line logic from Makefile
	// $(GCC) adk.c adk_go$(LIBEXT) -o _adk$(LIBEXT) $(CFLAGS) $(LDFLAGS) -fPIC --shared -w

	// We need to parse cflags/ldflags or just pass them as arguments to sh.Run
	// Note: python3-config output might contain multiple flags separated by space.
	// sh.Run args are individual tokens.

	// A simpler way is to use bash execution for this complex command line to avoid parsing issues
	// or reconstruct it carefully.

	// Let's rely on the nix environment variables if possible, or just the output of config.
	// The Makefile hardcoded some paths, but using python3-config is more portable.
	// However, to match the Makefile exactly (and the fix we made), we need to ensure RPATH is there.

	// -std=gnu17: gopy-generated C assumes the pre-C23 'bool'; see the
	// CGO_CFLAGS pin in the shell hook.
	linkCmdStr := fmt.Sprintf("gcc -std=gnu17 adk.c adk_go.so -o _adk.so %s %s -fPIC --shared -w -Wl,-rpath,$ORIGIN", cflags, ldflags)

	linkCmd := exec.Command("sh", "-c", linkCmdStr)
	linkCmd.Dir = nativeDir
	linkCmd.Stdout = os.Stdout
	linkCmd.Stderr = os.Stderr
	if err := linkCmd.Run(); err != nil {
		return fmt.Errorf("gcc link failed: %w", err)
	}

	fmt.Println("Python bindings built successfully")
	return nil
}
