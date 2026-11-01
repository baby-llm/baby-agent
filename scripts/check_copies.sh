#!/usr/bin/env bash
# Detect unintended drift between chapter snapshots.
#
# Chapters intentionally copy code from each other, but copies that are meant to
# stay identical must be patched everywhere (see AGENTS.md). This script
# normalizes package/import paths, hashes each listed copy, and fails when the
# hashes differ. Files with a deliberate per-chapter delta (engine.go, agent.go,
# tui.go, mcp.go, ...) are not listed.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# "<relative path>:<chapter> <chapter> ..."
groups=(
  "context/policy.go:ch05 ch06 ch08 ch09"
  "context/policy_offload.go:ch05 ch06 ch08 ch09"
  "context/policy_summary.go:ch05 ch06 ch08 ch09"
  "context/policy_truncate.go:ch05 ch06 ch08 ch09"
  "context/summary.go:ch05 ch06 ch08 ch09"
  "context/tokens.go:ch05 ch06 ch08 ch09"
  "storage/storage.go:ch05 ch06 ch08 ch09"
  "storage/filesystem.go:ch06 ch08 ch09"
  "storage/filesystem_test.go:ch06 ch08 ch09"
  "memory/update.go:ch06 ch08 ch09"
  "memory/update_test.go:ch06 ch08 ch09"
  "tool/bash.go:ch02 ch03 ch04 ch05 ch06 ch08 ch09"
  "tool/load_storage.go:ch05 ch06 ch08 ch09"
  "tool/docker_bash.go:ch08 ch09"
  "tool/docker_bash_test.go:ch08 ch09"
  "tool/factory.go:ch08 ch09"
  "mcp.go:ch04 ch05 ch06 ch08 ch09"
)

normalize() {
  sed -E 's/^package ch[0-9]+$/package chXX/; s|babyagent/ch[0-9]+|babyagent/chXX|g' "$1" | tr -d '\r'
}

failed=0
for group in "${groups[@]}"; do
  path="${group%%:*}"
  chapters="${group#*:}"
  reference=""
  reference_chapter=""

  for chapter in $chapters; do
    file="$chapter/$path"
    if [[ ! -f "$file" ]]; then
      echo "MISSING $file"
      failed=1
      continue
    fi
    hash="$(normalize "$file" | sha256sum | cut -d' ' -f1)"
    if [[ -z "$reference" ]]; then
      reference="$hash"
      reference_chapter="$chapter"
    elif [[ "$hash" != "$reference" ]]; then
      echo "DRIFT $file differs from $reference_chapter/$path"
      failed=1
    fi
  done
done

if [[ "$failed" -ne 0 ]]; then
  echo
  echo "Copies drifted. Patch every copy in the same change (AGENTS.md)."
  exit 1
fi

echo "All tracked chapter copies are in sync."
