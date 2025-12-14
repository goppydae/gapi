//go:build mage
// +build mage

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/goppydae/gapi/core/crypto"
)

var Default = BuildAll

const (
	version          = "0.1.0"
	goSDKVersion     = "0.1.0"
	pythonSDKVersion = "0.1.0"
	buildTag         = "dev"
	schemaHashFile   = "build/meta/.schema_hash"
)

// BuildAll compiles both binaries with embedded version info.
func BuildAll() error {
	if err := BuildCtl(); err != nil {
		return err
	}
	return BuildDaemon()
}

// BuildCtl builds the gapictl CLI tool.
func BuildCtl() error {
	fmt.Println("Building gapictl...")
	return buildBinary(
		"bin/gapictl",
		"./cmd/gapictl",
	)
}

// BuildDaemon builds the gapid supervisor binary.
func BuildDaemon() error {
	fmt.Println("Building gapid...")
	return buildBinary(
		"bin/gapid",
		"./cmd/gapid",
	)
}

// Shared binary build logic with linker flags and metadata output.
func buildBinary(outputBinary, mainPackage string) error {
	schemaHash := "dev"
	if hashBytes, err := os.ReadFile(schemaHashFile); err == nil {
		schemaHash = strings.TrimSpace(string(hashBytes))
	}

	commit := run("git", "rev-parse", "HEAD")
	date := run("date", "-u", "+%Y-%m-%dT%H:%M:%SZ")
	builtBy := os.Getenv("USER")

	ldflags := fmt.Sprintf(
		"-X 'github.com/goppydae/gapi/core/version.GAPIVersion=%s' "+
			"-X 'github.com/goppydae/gapi/core/version.GoSDKVersion=%s' "+
			"-X 'github.com/goppydae/gapi/core/version.PythonSDKVersion=%s' "+
			"-X 'github.com/goppydae/gapi/core/version.BuildTag=%s' "+
			"-X 'github.com/goppydae/gapi/core/version.SchemaHash=%s' "+
			"-X 'github.com/goppydae/gapi/core/version.Commit=%s' "+
			"-X 'github.com/goppydae/gapi/core/version.Date=%s' "+
			"-X 'github.com/goppydae/gapi/core/version.BuiltBy=%s'",
		version, goSDKVersion, pythonSDKVersion, buildTag,
		schemaHash, commit, date, builtBy,
	)

	cmd := exec.Command("go", "build", "-tags", "dev", "-ldflags", ldflags, "-o", outputBinary, mainPackage)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	meta := BuildMetadata{
		Version:          version,
		GoSDKVersion:     goSDKVersion,
		PythonSDKVersion: pythonSDKVersion,
		SchemaHash:       schemaHash,
		BuildTag:         buildTag,
		Commit:           commit,
		Date:             date,
		BuiltBy:          builtBy,
		OutputBinary:     outputBinary,
	}
	return writeBuildMeta(meta)
}

// Gen regenerates Protobuf files and stamps the schema hash.
func Gen() error {
	fmt.Println("Generating Protobuf files...")
	cmd := exec.Command("buf", "generate")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	fmt.Println("Stamping schema hash...")
	var allProto bytes.Buffer

	err := filepath.Walk("proto", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".proto") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			allProto.Write(data)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("collecting proto files: %w", err)
	}

	full, _ := crypto.Blake3.Hash(allProto.Bytes())
	err = os.WriteFile("build/meta/.schema_hash", []byte(full), 0644)
	if err != nil {
		return fmt.Errorf("writing schema hash: %w", err)
	}

	fmt.Printf("Schema hash: %s\n", full)
	return nil
}

// Tls generates self-signed TLS certs.
func Tls() error {
	dir := "config/certs"
	fmt.Println("Generating self-signed TLS certificate into", dir)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	cmd := exec.Command("openssl", "req",
		"-x509", "-newkey", "rsa:2048", "-nodes",
		"-keyout", dir+"/server.key",
		"-out", dir+"/server.crt",
		"-days", "365",
		"-subj", "/CN=localhost")

	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// BuildBindings generates the Python bindings using gopy.
func BuildBindings() error {
	fmt.Println("Generating native Python bindings...")
	// Ensure the output directory exists
	if err := os.MkdirAll("adk/python/gapi/native", 0755); err != nil {
		return err
	}

	// Build bindings for adk/go package
	// We use the manually installed gopy from .bin
	cmd := exec.Command("gopy", "build",
		"-output=adk/python/gapi/native",
		"-vm=python3",
		"github.com/goppydae/gapi/adk/go",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Dev starts live-reload using Air.
func Dev() error {
	fmt.Println("Running dev mode with Air...")
	cmd := exec.Command("air")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// run executes a shell command and returns trimmed output.
func run(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// BuildMetadata represents the structure written to .json metadata files.
type BuildMetadata struct {
	Version          string `json:"version"`
	GoSDKVersion     string `json:"go_sdk"`
	PythonSDKVersion string `json:"python_sdk"`
	SchemaHash       string `json:"schema_hash"`
	BuildTag         string `json:"build_tag"`
	Commit           string `json:"commit"`
	Date             string `json:"date"`
	BuiltBy          string `json:"built_by"`
	OutputBinary     string `json:"output_binary"`
}

// writeBuildMeta outputs metadata as JSON in build/meta/<binary>.json
func writeBuildMeta(meta BuildMetadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("build/buildmeta", 0755); err != nil {
		return err
	}
	filename := strings.Replace(meta.OutputBinary, "bin/", "build/buildmeta/", 1) + ".json"
	return ioutil.WriteFile(filename, data, 0644)
}
