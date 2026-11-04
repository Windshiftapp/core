#!/usr/bin/env bash
# Deprecated v1 API guard.
#
# The v1 REST API (/rest/api/v1) is deprecated and frozen: new integrations,
# clients, and tooling must use v2 (/rest/api/v2 for bearer tokens, /api/v2 for
# session clients; contract api/openapi-v2.json). This guard fails when
# first-party source names the v1 base path outside the few places that must:
#
#   internal/restapi/v1/            the v1 implementation itself
#   internal/restapi/router.go      mounts the v1 route group
#   frontend/.../features/api-docs/ the API docs viewer (labels v1 deprecated)
#   windshift-runner/main.go        error text naming the wrong base URL
#   scripts/check-deprecated-v1-api.sh, scripts/pre-commit   this guard
#
# It scans files in the git index, so it sees staged new files but ignores
# untracked scratch files. Comment lines are ignored, so documentation and
# rationale comments may name the path freely.
#
# Bypass: set V1_API_SKIP=1 (e.g. for emergency commits).

set -euo pipefail

if [[ "${V1_API_SKIP:-0}" == "1" ]]; then
    echo "  Deprecated v1 API guard skipped via V1_API_SKIP=1"
    exit 0
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

# Match the v1 base path but not a hypothetical v10.
PATTERN='rest/api/v1([^0-9]|$)'
# Drop comment lines: //, #, * and /* (Go block comments).
NOT_COMMENT=':[0-9]+:[[:space:]]*(//|#|\*|/\*)'

EXCLUDES=(
    'internal/restapi/v1/'
    'internal/restapi/router.go'
    'frontend/src/lib/features/api-docs/'
    'windshift-runner/main.go'
    'scripts/check-deprecated-v1-api.sh'
    'scripts/pre-commit'
)

files=()
while IFS= read -r path; do
    case "$path" in
        internal/*|cmd/*|windshift-runner/*|frontend/src/*|packages/*|scripts/*) ;;
        *) continue ;;
    esac
    skip=0
    for exclude in "${EXCLUDES[@]}"; do
        if [[ "$path" == "$exclude"* ]]; then
            skip=1
            break
        fi
    done
    [[ "$skip" -eq 1 ]] && continue
    case "$path" in
        *.go|*.ts|*.tsx|*.js|*.jsx|*.mjs|*.svelte) files+=("$path") ;;
    esac
done < <(git ls-files)

violations=()
if [[ "${#files[@]}" -gt 0 ]]; then
    while IFS= read -r match; do
        [[ -z "$match" ]] && continue
        violations+=("$match")
    done < <(grep -HnE "$PATTERN" "${files[@]}" 2>/dev/null | grep -vE "$NOT_COMMENT" || true)
fi

if [[ "${#violations[@]}" -gt 0 ]]; then
    echo "FAIL: the deprecated v1 API (/rest/api/v1) is used in new code:"
    printf '  %s\n' "${violations[@]}"
    echo
    echo "Use v2 instead: bearer clients call /rest/api/v2, session clients /api/v2"
    echo "(contract: api/openapi-v2.json). v1 is frozen and receives security fixes"
    echo "only; see the API version policy in AGENTS.md."
    echo
    echo "If a line legitimately names v1 (documentation, a guard, or an error"
    echo "message), make it a comment line or extend the allowlist in"
    echo "scripts/check-deprecated-v1-api.sh."
    exit 1
fi

echo "  Deprecated v1 API guard passed."
