#!/usr/bin/env bash
# ==============================================================================
# Nacho Flow Installer Test Suite
# Tests CLI flags, dry-run mode, help output, and parameter parsing.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_SCRIPT="${SCRIPT_DIR}/install.sh"

echo "=== Running Nacho Flow Installer Test Suite ==="

# Test 1: Help Flag
echo "Test 1: Testing --help flag..."
output="$(bash "$INSTALL_SCRIPT" --help)"
if ! echo "$output" | grep -q "Usage: install.sh"; then
    echo "FAIL: --help output does not contain usage information"
    exit 1
fi
echo "PASS: --help flag"

# Test 2: Dry Run Mode
echo "Test 2: Testing --dry-run mode..."
dry_run_output="$(bash "$INSTALL_SCRIPT" --dry-run --version v0.3.0)"
if ! echo "$dry_run_output" | grep -q "\[DRY-RUN\]"; then
    echo "FAIL: --dry-run did not produce DRY-RUN logs"
    exit 1
fi
if ! echo "$dry_run_output" | grep -q "v0.3.0"; then
    echo "FAIL: --dry-run did not recognize target version"
    exit 1
fi
echo "PASS: --dry-run mode"

# Test 3: Custom Directory in Dry Run
echo "Test 3: Testing --dir with --dry-run..."
dir_output="$(bash "$INSTALL_SCRIPT" --dry-run --dir /opt/custom/bin)"
if ! echo "$dir_output" | grep -q "/opt/custom/bin"; then
    echo "FAIL: custom directory not reflected in target path"
    exit 1
fi
echo "PASS: --dir custom directory option"

echo "=== All Installer Tests Passed Successfully! ==="
