#!/usr/bin/env bash
# Print current direct database-access metrics for production handler files.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

# Match direct DB methods while avoiding URL query parsing (`r.URL.Query()`).
# Plain `.Query(...)` is only counted when it has at least one argument.
PATTERN='\.(QueryRowContext|QueryContext|ExecWriteContext|ExecContext|QueryRow|ExecWrite|Exec|BeginTx|Begin)[[:space:]]*\(|\.Query[[:space:]]*\([[:space:]]*[^)]'

total_files=0
offending_files=0
total_ops=0
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    total_files=$((total_files + 1))

    import_count=0
    op_count=0
    if grep -q '"database/sql"' "$file"; then
        import_count=1
    fi
    op_count=$( (grep -Eo "$PATTERN" "$file" || true) | wc -l | tr -d '[:space:]' )

    if [[ "$import_count" -gt 0 || "$op_count" -gt 0 ]]; then
        offending_files=$((offending_files + 1))
        total_ops=$((total_ops + op_count))
        printf '%s\t%s\t%s\n' "$op_count" "$import_count" "$file" >> "$tmp"
    fi
done < <(find internal/handlers -name '*.go' ! -name '*_test.go' | sort)

echo "Production handler files: $total_files"
echo "Offending handler files: $offending_files"
echo "Direct DB operation occurrences: $total_ops"
echo
printf 'Top 30 files by direct DB operation count:\n'
printf '%8s  %10s  %s\n' "DB ops" "sql import" "File"
sort -rn "$tmp" | head -30 | awk -F '\t' '{ printf "%8d  %10s  %s\n", $1, ($2 == 1 ? "yes" : "no"), $3 }'
