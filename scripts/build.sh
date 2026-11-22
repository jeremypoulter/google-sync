#!/bin/sh
# Builds gmigrate for Windows and Linux with the family OAuth client built in.
#   scripts/build.sh path/to/client_secret.json [version]
# Keep the resulting binaries within the family: they contain the client secret.
set -eu
cd "$(dirname "$0")/.."
[ $# -ge 1 ] || { echo "usage: $0 CLIENT_JSON [VERSION]" >&2; exit 2; }
flags=$(go run ./scripts/ldflags "$1" "${2:-dev}")
mkdir -p dist
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$flags" -o dist/gmigrate.exe ./cmd/gmigrate
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$flags" -o dist/gmigrate ./cmd/gmigrate
echo "Built dist/gmigrate.exe and dist/gmigrate"
