// Hash agent for testing schema hashing
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/goppydae/gapi/core/crypto"
)

var describe = flag.Bool("describe", false, "Print agent metadata")

func main() {
	flag.Parse()

	if *describe {
		metadata := map[string]interface{}{
			"describe": map[string]interface{}{
				"id":           "hash_agent",
				"type":         "service",
				"version":      "1.0.0",
				"language":     "go",
				"description":  "Agent for testing schema hashing",
				"capabilities": []string{"compute_schema_hash"},
			},
		}
		json.NewEncoder(os.Stdout).Encode(metadata)
		return
	}

	// Expect filename as argument
	if len(flag.Args()) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: hash_agent <filename>")
		os.Exit(1)
	}

	filename := flag.Args()[0]
	hash, err := crypto.HashFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to hash file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(hash)
}
