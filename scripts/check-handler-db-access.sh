#!/usr/bin/env bash
# Guardrail for the handler boundary rule: production HTTP handlers must not
# import database/sql or call database query/exec/transaction methods directly.
# Existing files can be grandfathered in docs/architecture/handler-db-access-allowlist.txt.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

ALLOWLIST="docs/architecture/handler-db-access-allowlist.txt"
[[ -f "$ALLOWLIST" ]] || touch "$ALLOWLIST"

PATTERN='\.(QueryRowContext|QueryContext|ExecWriteContext|ExecContext|QueryRow|Query|ExecWrite|Exec|BeginTx|Begin)[[:space:]]*\('
violations=()

while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    if grep -qxF "$file" "$ALLOWLIST"; then
        continue
    fi

    has_import=0
    has_ops=0
    grep -q '"database/sql"' "$file" && has_import=1
    grep -Eq "$PATTERN" "$file" && has_ops=1

    if [[ "$has_import" -eq 1 || "$has_ops" -eq 1 ]]; then
        reasons=()
        [[ "$has_import" -eq 1 ]] && reasons+=("imports database/sql")
        [[ "$has_ops" -eq 1 ]] && reasons+=("calls direct DB methods")
        violations+=("$file (${reasons[*]})")
    fi
done < <(find internal/handlers -name '*.go' ! -name '*_test.go' | sort)

if [[ "${#violations[@]}" -gt 0 ]]; then
    echo "FAIL: handler DB access is forbidden outside $ALLOWLIST:" >&2
    printf '  %s\n' "${violations[@]}" >&2
    echo >&2
    echo "Move DB access into services/repositories, or add a temporary allowlist entry with an owner/removal plan." >&2
    exit 1
fi

echo "  Handler DB access guard passed."
