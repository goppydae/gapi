package main

import (
	"log"

	"github.com/goppydae/gapi/core/version"
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(statusCmd)

	version.SetBinaryNameAndVersion("gapictl", "0.1.0")
}

func main() {
	if err := Execute(); err != nil {
		log.Fatal(err)
	}
}
