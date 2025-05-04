package version

import (
	"fmt"
	"runtime"
)

var (
	GAPIVersion      = "dev" // set via -ldflags
	GoSDKVersion     = "dev"
	PythonSDKVersion = "dev"
	BuildTag         = "dev"
	SchemaHash       = "unknown"
	GoVersion        = runtime.Version()
	Platform         = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)

func Summary() string {
	return fmt.Sprintf(
		"GOPPY Stack Version:   %s\n"+
			"GAPI Core Version:     %s\n"+
			"Go SDK Version:        %s\n"+
			"Python SDK Version:    %s\n"+
			"Protobuf Schema Hash:  %s\n"+
			"Build Info:            %s %s\n",
		GAPIVersion,
		GAPIVersion,
		GoSDKVersion,
		PythonSDKVersion,
		SchemaHash,
		BuildTag, Platform,
	)
}
