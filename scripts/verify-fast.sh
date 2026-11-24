#!/usr/bin/env bash
# Fast gate for pre-commit: checks only the files this commit touches. Target: under 60 seconds.
# Run from anywhere: bash scripts/verify-fast.sh (or make verify-fast).
#
# VERIFY_BASE=<git revision> adds every file changed since that revision to the check set.
# Use it on a clean checkout, where there is no working diff yet.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root" || exit 1

fail=0
note() { printf '  %s\n' "$*"; }
section() { printf '\n[%s]\n' "$*"; }

# Added, copied, modified or renamed files: working tree changes and staged changes together.
changed_files() {
  {
    if [ -n "${VERIFY_BASE:-}" ]; then
      git diff --name-only --diff-filter=ACMR "${VERIFY_BASE}" 2>/dev/null || true
    fi
    git diff --name-only --diff-filter=ACMR 2>/dev/null || true
    git diff --name-only --diff-filter=ACMR --cached 2>/dev/null || true
  } | sort -u
}

mapfile -t files < <(changed_files)
mapfile -t go_files < <(printf '%s\n' "${files[@]}" | grep -E '\.go$' || true)
mapfile -t go_mod_files < <(printf '%s\n' "${files[@]}" | grep -Ex 'go\.(mod|sum)' || true)
mapfile -t yaml_files < <(printf '%s\n' "${files[@]}" | grep -E '\.ya?ml$' || true)
mapfile -t shell_files < <(printf '%s\n' "${files[@]}" | grep -E '\.sh$' || true)
mapfile -t docker_files < <(printf '%s\n' "${files[@]}" | grep -E '(^|/)Dockerfile' || true)
mapfile -t json_files < <(printf '%s\n' "${files[@]}" | grep -E '\.json$' || true)

section "git whitespace"
git diff --check || fail=1
git diff --cached --check || fail=1

section "secrets (staged)"
if command -v gitleaks >/dev/null 2>&1; then
  gitleaks git --pre-commit --staged --config .gitleaks.toml --no-banner --redact --exit-code 1 || fail=1
else
  note "skip: gitleaks is not installed"
fi

section "go"
if [ "${#go_files[@]}" -eq 0 ] && [ "${#go_mod_files[@]}" -eq 0 ]; then
  note "skip: no Go files changed"
fi
if [ "${#go_files[@]}" -gt 0 ]; then
  # gofmt lists files that need formatting; a file that does not parse shows up here as an error.
  gofmt_output="$(gofmt -l "${go_files[@]}" 2>&1)"
  if [ -n "${gofmt_output}" ]; then
    note "gofmt needed:"
    printf '%s\n' "${gofmt_output}" | sed 's/^/    /'
    fail=1
  fi
  if command -v golangci-lint >/dev/null 2>&1; then
    golangci-lint run --new-from-rev="${VERIFY_BASE:-HEAD}" ./... || fail=1
  else
    note "skip: golangci-lint is not installed"
  fi
fi
if [ "${#go_mod_files[@]}" -gt 0 ]; then
  go mod tidy -diff || fail=1
fi

section "yaml"
if [ "${#yaml_files[@]}" -gt 0 ]; then
  if command -v yamllint >/dev/null 2>&1; then
    yamllint --strict "${yaml_files[@]}" || fail=1
  else
    note "skip: yamllint is not installed"
  fi
else
  note "skip: no YAML files changed"
fi

section "shell"
if [ "${#shell_files[@]}" -gt 0 ]; then
  if command -v shellcheck >/dev/null 2>&1; then
    shellcheck -x "${shell_files[@]}" || fail=1
  else
    note "skip: shellcheck is not installed"
  fi
else
  note "skip: no shell scripts changed"
fi

section "dockerfile"
if [ "${#docker_files[@]}" -gt 0 ]; then
  if command -v hadolint >/dev/null 2>&1; then
    hadolint "${docker_files[@]}" || fail=1
  else
    note "skip: hadolint is not installed"
  fi
else
  note "skip: no Dockerfile changed"
fi

section "json"
if [ "${#json_files[@]}" -gt 0 ]; then
  if command -v jq >/dev/null 2>&1; then
    for file in "${json_files[@]}"; do
      jq empty "${file}" || fail=1
    done
  else
    note "skip: jq is not installed"
  fi
else
  note "skip: no JSON files changed"
fi

if [ "${fail}" -ne 0 ]; then
  printf '\nverify-fast: FAILED\n'
  exit 1
fi
printf '\nverify-fast: OK\n'
