#!/bin/bash
# Prepares a Claude Code on the web container so `make check-q`, the E2E suite
# (e2e/, testcontainers Postgres) and the frontend build work without manual
# troubleshooting. Idempotent: every step checks before acting.
set -euo pipefail

if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

log() { echo "session-start: $*" >&2; }

# Set by Claude Code; derived from the script's location when run by hand.
CLAUDE_PROJECT_DIR="${CLAUDE_PROJECT_DIR:-$(cd "$(dirname "$0")/../.." && pwd)}"

# libvips: pkg/media links govips through cgo, so nothing that imports it
# (cmd/web, e2e) compiles without the headers.
if ! pkg-config --exists vips 2>/dev/null; then
  log "installing libvips-dev"
  export DEBIAN_FRONTEND=noninteractive
  apt-get install -y -q libvips-dev >/dev/null 2>&1 \
    || { apt-get update -q >/dev/null 2>&1 && apt-get install -y -q libvips-dev >/dev/null; }
fi

# Docker daemon: testdb and the E2E harness start Postgres with testcontainers.
# The binary is installed in the image but the daemon is not running.
if ! docker info >/dev/null 2>&1; then
  log "starting dockerd"
  setsid nohup dockerd >/tmp/dockerd.log 2>&1 < /dev/null &
  for _ in $(seq 1 30); do
    docker info >/dev/null 2>&1 && break
    sleep 1
  done
  docker info >/dev/null 2>&1 || { log "dockerd did not start, see /tmp/dockerd.log"; exit 1; }
fi

# Frontend dependencies for `make test-ui` (yarn --cwd cmd/web build).
if [ ! -d "$CLAUDE_PROJECT_DIR/cmd/web/node_modules" ]; then
  log "installing frontend dependencies"
  yarn --cwd "$CLAUDE_PROJECT_DIR/cmd/web" install --frozen-lockfile >/dev/null 2>&1 \
    || yarn --cwd "$CLAUDE_PROJECT_DIR/cmd/web" install >/dev/null
fi

# Go modules, so the first build doesn't spend minutes downloading.
(cd "$CLAUDE_PROJECT_DIR" && go mod download)

# covdata: `make cover` merges unit and E2E coverage with `go tool covdata`,
# which the image's Go install leaves out. It builds from the Go source tree.
tooldir=$(go env GOTOOLDIR)
if [ ! -x "$tooldir/covdata" ]; then
  log "building go tool covdata"
  go build -o "$tooldir/covdata" cmd/covdata
fi

# golangci-lint at CI's version. The image ships an older one, built with a Go
# older than go.mod's, which refuses to load the config.
want=$(sed -n 's/.*GOLANGCI_LINT_VERSION: *"v\([^"]*\)".*/\1/p' "$CLAUDE_PROJECT_DIR/.github/workflows/ci.yml")
if [ -n "$want" ] && ! golangci-lint version 2>/dev/null | grep -q "version $want "; then
  log "installing golangci-lint v$want"
  GOBIN=/usr/local/bin go install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$want"
fi

# Browser tests: the playwright-go driver downloads fine, but Playwright's CDN
# is blocked, so `make ui-deps` can't fetch Chromium. Use the image's Chromium.
(cd "$CLAUDE_PROJECT_DIR" && go run github.com/mxschmitt/playwright-go/cmd/playwright --version >/dev/null)
if [ -x /opt/pw-browsers/chromium ] && [ -n "${CLAUDE_ENV_FILE:-}" ]; then
  echo 'export CHROMIUM_PATH=/opt/pw-browsers/chromium' >> "$CLAUDE_ENV_FILE"
fi

log "ready"
