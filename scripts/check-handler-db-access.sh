#!/usr/bin/env bash
# Guardrail for the handler boundary rule: production HTTP handlers must not
# import database/sql or call database query/exec/transaction methods directly.
# Existing files can be grandfathered in scripts/.handler-db-access-allowlist.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

ALLOWLIST="scripts/.handler-db-access-allowlist"
[[ -f "$ALLOWLIST" ]] || touch "$ALLOWLIST"

HANDLER_DIRS=(
    "internal/handlers"
    "internal/restapi/v1/handlers"
    "internal/restapi/v2"
)

# Match direct DB methods while avoiding URL query parsing (`r.URL.Query()`).
# Plain `.Query(...)` is only counted when it has at least one argument.
PATTERN='\.(QueryRowContext|QueryContext|ExecWriteContext|ExecContext|QueryRow|ExecWrite|Exec|BeginTx|Begin)[[:space:]]*\(|\.Query[[:space:]]*\([[:space:]]*[^)]'
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
done < <(find "${HANDLER_DIRS[@]}" -name '*.go' ! -name '*_test.go' | sort)

if [[ "${#violations[@]}" -gt 0 ]]; then
    echo "FAIL: handler DB access is forbidden outside $ALLOWLIST:" >&2
    printf '  %s\n' "${violations[@]}" >&2
    echo >&2
    echo "Move DB access into services/repositories, or add a temporary allowlist entry with an owner/removal plan." >&2
    exit 1
fi

echo "  Handler DB access guard passed."
