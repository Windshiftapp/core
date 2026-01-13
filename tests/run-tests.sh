#!/bin/bash
# Run tests and generate HTML report
#
# Usage:
#   ./tests/run-tests.sh              # Run all tests
#   ./tests/run-tests.sh -run Pattern # Run specific tests
#   ./tests/run-tests.sh -v           # Verbose output

set -e

# Get the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Report output directory
REPORT_DIR="$SCRIPT_DIR/reports"
REPORT_FILE="$REPORT_DIR/test-report.html"
JSON_OUTPUT="$REPORT_DIR/test-output.json"

# Colors for terminal output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create report directory
mkdir -p "$REPORT_DIR"

# Clean up stale test database files from previous interrupted runs
# Only removes files older than 5 minutes to avoid removing DBs from concurrent tests
echo "Cleaning up stale test databases..."
find "$PROJECT_ROOT" -maxdepth 1 -name "test_*.db*" -type f -mmin +5 -delete 2>/dev/null || true
find "$SCRIPT_DIR" -maxdepth 1 -name "test_*.db*" -type f -mmin +5 -delete 2>/dev/null || true
# Also clean temp directory
find "${TMPDIR:-/tmp}" -maxdepth 2 -name "windshift-tests" -type d -mmin +30 -exec rm -rf {} \; 2>/dev/null || true

echo "Running tests..."
echo ""

# Change to project root for go test
cd "$PROJECT_ROOT"

# Build the report generator first
go build -o "$SCRIPT_DIR/cmd/testreport/testreport" "$SCRIPT_DIR/cmd/testreport/main.go"

# Run tests with JSON output
# Pass through any arguments (like -run Pattern, -v, etc.)
# Only run the main tests package (exclude stress tests which have build issues)
set +e  # Don't exit on test failure
go test -json ./tests "$@" 2>&1 | tee "$JSON_OUTPUT" | "$SCRIPT_DIR/cmd/testreport/testreport" > "$REPORT_FILE"
TEST_EXIT_CODE=${PIPESTATUS[0]}
set -e

# Simple count from JSON (count test-level pass/fail, not package level)
PASSED=$(grep '"Action":"pass"' "$JSON_OUTPUT" 2>/dev/null | grep '"Test":' | wc -l | tr -d ' ')
FAILED=$(grep '"Action":"fail"' "$JSON_OUTPUT" 2>/dev/null | grep '"Test":' | wc -l | tr -d ' ')

# Ensure we have valid integers (default to 0)
PASSED=$((PASSED + 0))
FAILED=$((FAILED + 0))

echo ""
echo "========================================"
echo "Test Results Summary"
echo "========================================"

if [ "$FAILED" -gt 0 ]; then
    echo -e "${RED}FAILED${NC}"
    echo -e "  Passed: ${GREEN}$PASSED${NC}"
    echo -e "  Failed: ${RED}$FAILED${NC}"
else
    echo -e "${GREEN}PASSED${NC}"
    echo -e "  Passed: ${GREEN}$PASSED${NC}"
fi

echo ""
echo "HTML Report: $REPORT_FILE"
echo "JSON Output: $JSON_OUTPUT"
echo ""

# Open report in browser (macOS) - don't fail if open command fails
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Opening report in browser..."
    open "$REPORT_FILE" 2>/dev/null || true
fi

exit $TEST_EXIT_CODE
