#!/usr/bin/env bash
# Runs govulncheck and fails on any vulnerability that is not explicitly
# accepted below.
#
# govulncheck has no built-in ignore list, so this wrapper extracts the OSV ids
# from its output and compares them with ACCEPTED_VULNERABILITIES. Keep the
# accepted list empty when possible. Every entry must name the reason and the
# version that fixes it, and must be removed once the Go vulnerability database
# records a fixed version (then plain `govulncheck ./...` passes on its own).
set -uo pipefail

# Accepted findings. Each entry is a bare OSV id; the justification lives in the
# comment beside it.
ACCEPTED_VULNERABILITIES=(
  # crewjam/saml namespace-blind Subject.NameID and nil Subject/Conditions.
  # Fixed in v0.5.2 (namespace-qualified XML tags); the vuln DB entry has not
  # yet recorded a fixed version, so govulncheck still reports it.
  "GO-2026-6909"
)

if ! command -v govulncheck >/dev/null 2>&1; then
  echo "govulncheck not installed; run 'make install-govulncheck'." >&2
  exit 1
fi

output="$(govulncheck ./... 2>&1)" && {
  echo "govulncheck: no vulnerabilities found."
  exit 0
}

reported="$(printf '%s\n' "$output" | grep -oE 'GO-[0-9]{4}-[0-9]+' | sort -u || true)"
if [ -z "$reported" ]; then
  echo "govulncheck failed without reporting a vulnerability id:" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi

unexpected=""
while IFS= read -r id; do
  [ -z "$id" ] && continue
  accepted=0
  for accepted_id in "${ACCEPTED_VULNERABILITIES[@]}"; do
    if [ "$id" = "$accepted_id" ]; then
      accepted=1
      break
    fi
  done
  if [ "$accepted" -eq 1 ]; then
    echo "govulncheck: accepted risk $id"
  else
    unexpected="$unexpected $id"
  fi
done <<< "$reported"

if [ -n "$unexpected" ]; then
  echo "govulncheck: unaccepted vulnerabilities:$unexpected" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi

echo "govulncheck: no unaccepted vulnerabilities."
