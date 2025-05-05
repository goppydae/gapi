package main

import (
	"github.com/rs/zerolog"
	
	"github.com/goppydae/gapi/core/version"
	"github.com/goppydae/gapi/internal/logging/logcore"
)

func init() {
	logcore.Init(zerolog.InfoLevel)
	rootCmd.AddCommand(versionCmd)

	version.SetBinaryNameAndVersion("gapid", "0.1.0")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		logcore.Fatal().Err(err).Msg("command execution failed")
	}
}
