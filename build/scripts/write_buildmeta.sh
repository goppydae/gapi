#!/usr/bin/env bash
set -e

# Accept values via env vars
mkdir -p build/meta

cat > build/meta/$OUTPUT_FILE.json <<EOF
{
  "version": "$VERSION",
  "go_ddk": "$GODDK",
  "python_ddk": "$PYDDK",
  "schema_hash": "$SCHEMA",
  "build_tag": "$TAG",
  "commit": "$COMMIT",
  "date": "$DATE",
  "built_by": "$USER",
  "output_binary": "$OUTPUT_BIN"
}
EOF
