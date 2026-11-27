#!/usr/bin/env bash
# Print current direct database-access metrics for production handler files.

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

HANDLER_DIRS=(
    "internal/handlers"
    "internal/restapi/v1/handlers"
)

# Match direct DB methods while avoiding URL query parsing (`r.URL.Query()`).
# Plain `.Query(...)` is only counted when it has at least one argument.
PATTERN='\.(QueryWriteContext|QueryWrite|QueryRowWriteContext|QueryRowWrite|QueryReadContext|QueryRowReadContext|QueryRowContext|QueryContext|ExecWriteContext|ExecContext|QueryRow|ExecWrite|Exec|BeginTx|Begin)[[:space:]]*\(|\.Query[[:space:]]*\([[:space:]]*[^)]'

total_files=0
offending_files=0
total_ops=0
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

surface_for_file() {
    case "$1" in
        internal/restapi/v1/handlers/*) echo "restapi/v1" ;;
        internal/handlers/*) echo "legacy" ;;
        *) echo "unknown" ;;
    esac
}

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
        printf '%s\t%s\t%s\t%s\n' "$op_count" "$import_count" "$(surface_for_file "$file")" "$file" >> "$tmp"
    fi
done < <(find "${HANDLER_DIRS[@]}" -name '*.go' ! -name '*_test.go' | sort)

echo "Production handler files: $total_files"
echo "Offending handler files: $offending_files"
echo "Direct DB operation occurrences: $total_ops"
echo
printf 'By surface:\n'
printf '%-12s  %8s  %8s\n' "Surface" "Files" "DB ops"
awk -F '\t' '
    { files[$3] += 1; ops[$3] += $1 }
    END { for (surface in files) printf "%-12s  %8d  %8d\n", surface, files[surface], ops[surface] }
' "$tmp" | sort

echo
printf 'Top 30 files by direct DB operation count:\n'
printf '%8s  %10s  %-10s  %s\n' "DB ops" "sql import" "Surface" "File"
sort -rn "$tmp" | head -30 | awk -F '\t' '{ printf "%8d  %10s  %-10s  %s\n", $1, ($2 == 1 ? "yes" : "no"), $3, $4 }'
