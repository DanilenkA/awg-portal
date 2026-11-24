#!/usr/bin/env bash
# Full gate: run it before handing work over; lefthook runs it on pre-push. Target: under 5 minutes.
# Run from anywhere: bash scripts/verify-full.sh (or make verify-full).
#
# VERIFY_BASE=<git revision> limits the golangci-lint run to code changed since that revision.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root" || exit 1

fail=0
note() { printf '  %s\n' "$*"; }
section() { printf '\n[%s]\n' "$*"; }

govulncheck_bin() {
  if command -v govulncheck >/dev/null 2>&1; then
    command -v govulncheck
  elif [ -x "${HOME}/go/bin/govulncheck" ]; then
    printf '%s\n' "${HOME}/go/bin/govulncheck"
  else
    return 1
  fi
}

section "verify-fast"
bash scripts/verify-fast.sh || fail=1

section "go format"
# golangci-lint fmt exits with 1 when any file would change and prints the diff.
golangci-lint fmt --diff ./... || fail=1

section "go vet"
go vet ./... || fail=1

section "go build"
go build ./... || fail=1

section "go test"
go test -race -short ./... || fail=1

section "golangci-lint"
golangci-lint run --new-from-rev="${VERIFY_BASE:-main}" ./... || fail=1

section "govulncheck"
if govulncheck_bin="$(govulncheck_bin)"; then
  "${govulncheck_bin}" ./... || fail=1
else
  note "skip: govulncheck is not installed (go install golang.org/x/vuln/cmd/govulncheck@v1.10.0)"
fi

section "frontend build"
if [ ! -d frontend/node_modules ]; then
  note "skip: frontend/node_modules is missing (cd frontend && npm ci)"
elif ! command -v npm >/dev/null 2>&1; then
  note "skip: npm is not installed"
else
  # The build output is embedded into the Go binary.
  (cd frontend && npm run build) || fail=1
fi

if [ "${fail}" -ne 0 ]; then
  printf '\nverify-full: FAILED\n'
  exit 1
fi
printf '\nverify-full: OK\n'
