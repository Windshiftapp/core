#!/usr/bin/env bash
# Layering guard: enforces architectural import rules that go beyond what
# golangci-lint catches.
#
# Each rule is `<package_glob>::<forbidden_import_path>::<rationale>`.
# A rule fires when any non-test .go file in the package matches the glob and
# imports the forbidden path. Failures print file:line and the rationale.
#
# Existing violations are documented in .layering-baseline so we can land this
# guard without a giant migration. Anything outside the baseline fails the
# build. To remove an entry from the baseline, fix the violation; to add one,
# explain in the commit why.
#
# Bypass: set LAYERING_SKIP=1 (e.g. for emergency commits).

set -euo pipefail

if [[ "${LAYERING_SKIP:-0}" == "1" ]]; then
    echo "  Layering guard skipped via LAYERING_SKIP=1"
    exit 0
fi

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

BASELINE_FILE="scripts/.layering-baseline"
[[ -f "$BASELINE_FILE" ]] || touch "$BASELINE_FILE"

# rule format: <package>|<forbidden>|<rationale>
# package is matched against the relative dir (e.g. internal/auth)
RULES=(
    "internal/auth|windshift/internal/services|auth is a foundational primitive; services should depend on auth, not the reverse"
    "internal/cacheutil|windshift/internal|cacheutil must stay a leaf (only stdlib + 3rd-party deps)"
    "internal/database|windshift/internal/emailutil|database is infrastructure; domain helpers should be injected, not imported"
    "internal/handlers|windshift/internal/database|handlers must go through repositories, not raw database access"
)

found=0
new_violations=()

for rule in "${RULES[@]}"; do
    pkg="${rule%%|*}"
    rest="${rule#*|}"
    forbidden="${rest%%|*}"
    rationale="${rest#*|}"

    [[ -d "$pkg" ]] || continue

    while IFS= read -r match; do
        [[ -z "$match" ]] && continue
        # match looks like: internal/handlers/foo.go:12:	"windshift/internal/database"
        file="${match%%:*}"
        # baseline key: file|forbidden
        key="${file}|${forbidden}"
        if grep -Fxq "$key" "$BASELINE_FILE"; then
            continue # grandfathered
        fi
        new_violations+=("$match  -- $rationale")
        found=1
    done < <(grep -rn "\"$forbidden\"" "$pkg" --include='*.go' 2>/dev/null | grep -v '_test\.go' || true)
done

if [[ "$found" -eq 1 ]]; then
    echo "FAIL: layering guard found new architectural import violations:"
    printf '  %s\n' "${new_violations[@]}"
    echo
    echo "Either fix the import (see scripts/check-layering.sh for rationale)"
    echo "or, if intentional, add an entry to $BASELINE_FILE in the same commit"
    echo "and explain why in the commit message."
    exit 1
fi

echo "  Layering guard passed."
