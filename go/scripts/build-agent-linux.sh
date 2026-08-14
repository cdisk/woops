#!/usr/bin/env bash
# Build stripped Linux agent + gzip sidecar.
# Version format (dev phase): yymmddhhMM local time. Later can switch to yymmdd.
# Prefer running inside a Linux container (host GOOS=linux cross-build has segfaulted).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export CGO_ENABLED=0

VERSION="${OPS_AGENT_VERSION:-$(date +%y%m%d%H%M)}"
echo "Building woops-agent version=$VERSION"

mkdir -p bin
# Full import path avoids flaky "no Go files in /src" with some volume mounts.
go build -ldflags="-s -w -X main.Version=${VERSION}" -trimpath -o bin/woops-agent-linux-amd64 github.com/ops-bastion/ops/go/cmd/agent
gzip -c -9 bin/woops-agent-linux-amd64 > bin/woops-agent-linux-amd64.gz
ls -la bin/woops-agent-linux-amd64 bin/woops-agent-linux-amd64.gz
echo "version: $VERSION"
