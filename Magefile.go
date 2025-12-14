//go:build mage
// +build mage

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Build builds the gapid and gapictl binaries
func Build() error {
	mg.Deps(ensureGCC)
	fmt.Println("Building gapid and gapictl...")

	if err := sh.Run("go", "build", "-o", "bin/gapid", "./cmd/gapid"); err != nil {
		return err
	}

	if err := sh.Run("go", "build", "-o", "bin/gapictl", "./cmd/gapictl"); err != nil {
		return err
	}

	fmt.Println("✅ Build complete: bin/gapid, bin/gapictl")
	return nil
}

// Install installs binaries to $GOPATH/bin
func Install() error {
	mg.Deps(ensureGCC)
	fmt.Println("Installing gapid and gapictl...")

	if err := sh.Run("go", "install", "./cmd/gapid"); err != nil {
		return err
	}

	if err := sh.Run("go", "install", "./cmd/gapictl"); err != nil {
		return err
	}

	fmt.Println("✅ Installed to $GOPATH/bin")
	return nil
}

// Test runs all tests
func Test() error {
	mg.Deps(ensureGCC)
	fmt.Println("Running tests...")
	return sh.RunV("go", "test", "-v", "./...")
}

// TestUnit runs only unit tests
func TestUnit() error {
	mg.Deps(ensureGCC)
	fmt.Println("Running unit tests...")
	return sh.RunV("go", "test", "-v", "./internal/...")
}

// TestADK runs ADK integration tests
func TestADK() error {
	mg.Deps(Build)
	fmt.Println("Running ADK integration tests...")
	return sh.RunV("go", "test", "-v", "./test/adk/...")
}

// TestE2E runs end-to-end tests
func TestE2E() error {
	mg.Deps(Build)
	fmt.Println("Running E2E tests...")
	return sh.RunV("./test/e2e.sh")
}

// Clean removes build artifacts
func Clean() error {
	fmt.Println("Cleaning build artifacts...")

	dirs := []string{"bin", "build"}
	for _, dir := range dirs {
		if err := sh.Rm(dir); err != nil {
			fmt.Printf("Warning: failed to remove %s: %v\n", dir, err)
		}
	}

	fmt.Println("✅ Clean complete")
	return nil
}

// Proto generates protobuf code
func Proto() error {
	fmt.Println("Generating protobuf code...")

	protoFiles, err := filepath.Glob("proto/*.proto")
	if err != nil {
		return err
	}

	for _, file := range protoFiles {
		args := []string{
			"--go_out=.",
			"--go_opt=paths=source_relative",
			"--go-grpc_out=.",
			"--go-grpc_opt=paths=source_relative",
			file,
		}
		if err := sh.Run("protoc", args...); err != nil {
			return fmt.Errorf("protoc failed for %s: %w", file, err)
		}
	}

	fmt.Println("✅ Protobuf generation complete")
	return nil
}

// Fmt formats all Go code
func Fmt() error {
	fmt.Println("Formatting code...")
	return sh.RunV("go", "fmt", "./...")
}

// Lint runs linters
func Lint() error {
	fmt.Println("Running linters...")

	// Check if golangci-lint is available
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		return sh.RunV("golangci-lint", "run")
	}

	// Fallback to go vet
	fmt.Println("golangci-lint not found, using go vet...")
	return sh.RunV("go", "vet", "./...")
}

// Tidy runs go mod tidy
func Tidy() error {
	fmt.Println("Tidying go.mod...")
	return sh.Run("go", "mod", "tidy")
}

// Dev runs the development build and starts gapid
func Dev() error {
	mg.Deps(Build)
	fmt.Println("Starting gapid in development mode...")
	return sh.RunV("./bin/gapid")
}

// All runs fmt, tidy, build, and test
func All() error {
	mg.Deps(Fmt, Tidy, Build, Test)
	fmt.Println("✅ All tasks complete")
	return nil
}

// ensureGCC checks if gcc is available and provides helpful error
func ensureGCC() error {
	if _, err := exec.LookPath("gcc"); err != nil {
		return fmt.Errorf(`gcc not found in PATH

Please ensure you're in the nix development shell:
  nix develop

Or run mage commands through nix:
  nix develop -c mage build

Error: %w`, err)
	}
	return nil
}
