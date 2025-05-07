#!/usr/bin/env bash
set -e

# Check for --build-only mode
BUILD_ONLY=false
if [[ "$1" == "--build-only" ]]; then
    BUILD_ONLY=true
fi

# Shared version variables
export VERSION=dev
export GODDK=dev
export PYDDK=dev
export TAG=dev
export SCHEMA=$(cat build/meta/.schema_hash 2>/dev/null || echo "dev")
export COMMIT=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
export DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
export USER=$(whoami)

# Shared linker flags
LD_FLAGS="
-X 'github.com/goppydae/gapi/core/version.GAPIVersion=$VERSION' \
-X 'github.com/goppydae/gapi/core/version.GoDDKVersion=$GODDK' \
-X 'github.com/goppydae/gapi/core/version.PythonDDKVersion=$PYDDK' \
-X 'github.com/goppydae/gapi/core/version.BuildTag=$TAG' \
-X 'github.com/goppydae/gapi/core/version.SchemaHash=$SCHEMA' \
-X 'github.com/goppydae/gapi/core/version.Commit=$COMMIT' \
-X 'github.com/goppydae/gapi/core/version.Date=$DATE' \
-X 'github.com/goppydae/gapi/core/version.BuiltBy=$USER'
"

# Build gapictl
echo "Building gapictl..."
go build -ldflags "$LD_FLAGS" -tags dev -o bin/gapictl ./cmd/gapictl
chmod +x bin/gapictl
export OUTPUT_BIN=bin/gapictl
export OUTPUT_FILE=gapictl
./build/scripts/write_buildmeta.sh

# Build gapid
echo "Building gapid..."
go build -ldflags "$LD_FLAGS" -tags dev -o bin/gapid ./cmd/gapid
export OUTPUT_BIN=bin/gapid
export OUTPUT_FILE=gapid
./build/scripts/write_buildmeta.sh

# Conditionally run gapid if not in build-only mode
if [ "$BUILD_ONLY" = false ]; then
    echo "Launching gapid..."
    exec ./bin/gapid
else
    echo "Build completed successfully (build-only mode)."
fi
