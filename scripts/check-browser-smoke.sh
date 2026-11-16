#!/usr/bin/env bash
set -euo pipefail

# Production module initialization needs a real browser, not just a successful build.
REPO_ROOT="$(git rev-parse --show-toplevel)"
REQUIRED=0
while IFS= read -r -d '' path; do
    case "$path" in
        .nvmrc|frontend/package.json|frontend/package-lock.json|frontend/.npmrc|\
        frontend/vite*.js|frontend/vite*.ts|frontend/svelte.config.*|frontend/index.html|\
        frontend/scripts/check-entry-assets.js|frontend/src/main.*|frontend/src/App.svelte|\
        frontend/src/lib/router.js|frontend/src/lib/runtime/*|\
        frontend/src/lib/pages/MainApp.svelte|frontend/src/lib/pages/MainRouteContent.svelte|\
        frontend/src/lib/pages/mainAppRoutes.js|frontend/src/lib/services/authenticatedShellUI.js|\
        frontend/src/lib/utils/lazy*|frontend/src/lib/components/Lazy*|\
        frontend/src/lib/editors/*|frontend/src/lib/features/items/ItemDetailDescription.svelte|\
        frontend/src/lib/features/items/Comments.svelte|\
        scripts/pre-commit|scripts/check-browser-smoke.sh)
            REQUIRED=1
            ;;
    esac
done < <(git -C "$REPO_ROOT" diff --cached --name-only --no-renames -z)

if [[ "$REQUIRED" -eq 0 && -z "${BROWSER_SPECS:-}" ]]; then
    echo 'Production browser smoke: skipped (no staged build, loading, or editor changes).'
    exit 0
fi

TESTS_DIR="${CORE_TESTS_DIR:-$REPO_ROOT/../core-tests}"
if [[ ! -x "$TESTS_DIR/run-e2e.sh" ]]; then
    echo 'FAIL: production browser smoke requires the private core-tests checkout.' >&2
    echo 'Clone it beside core, or set CORE_TESTS_DIR to its absolute path.' >&2
    exit 1
fi
TESTS_DIR="$(cd "$TESTS_DIR" && pwd)"

if [[ -z "${BROWSER_SPECS:-}" ]]; then
    echo 'FAIL: select focused production-browser specs for this change.' >&2
    echo 'Set BROWSER_SPECS to space-separated spec paths when committing, for example:' >&2
    echo 'BROWSER_SPECS="tests/item-description-markdown-view.spec.ts" git commit ...' >&2
    exit 1
fi
read -r -a SPECS <<< "${BROWSER_SPECS//$'\n'/ }"
if [[ "${#SPECS[@]}" -eq 0 ]]; then
    echo 'FAIL: BROWSER_SPECS must select at least one spec.' >&2
    exit 1
fi
for spec in "${SPECS[@]}"; do
    case "$spec" in
        tests/*.spec.ts) ;;
        *) echo "FAIL: expected an explicit tests/*.spec.ts path, got: $spec" >&2; exit 1 ;;
    esac
    if [[ "$spec" == *..* || ! -f "$TESTS_DIR/e2e/$spec" ]]; then
        echo "FAIL: browser spec not found in core-tests: $spec" >&2
        exit 1
    fi
done

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/windshift-browser-smoke-XXXXX")"
PASSED=0
cleanup() {
    if [[ "$PASSED" -eq 1 ]]; then
        rm -rf "$WORK_DIR"
    else
        echo "Production browser smoke failed; staged checkout and server logs retained at $WORK_DIR" >&2
        echo "Playwright failure artifacts are retained under $TESTS_DIR/e2e/." >&2
    fi
}
trap cleanup EXIT

# An unstaged local fix must not make a broken staged commit appear healthy.
mkdir "$WORK_DIR/core"
git -C "$REPO_ROOT" archive "$(git -C "$REPO_ROOT" write-tree)" | tar -xf - -C "$WORK_DIR/core"
echo "Running selected production-browser specs against the staged checkout: ${SPECS[*]}"
(
    unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_PREFIX
    CORE_DIR="$WORK_DIR/core" E2E_KEEP_ARTIFACTS=1 \
        E2E_FAIL_ON_UNEXPECTED_SKIP=1 E2E_FAIL_ON_RETRY=1 \
        "$TESTS_DIR/run-e2e.sh" "${SPECS[@]}"
)
PASSED=1
