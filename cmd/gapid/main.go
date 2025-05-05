package main

import (
	"fmt"
	"os"

	"github.com/goppydae/gapi/core/version"
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)

	version.SetBinaryNameAndVersion("gapid", "0.1.0")
}

func main() {
	if err := Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
