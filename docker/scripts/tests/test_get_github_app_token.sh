#!/usr/bin/env bats
# Tests for docker/scripts/get-github-app-token.sh
#
# Run with:  bats docker/scripts/tests/test_get_github_app_token.sh
#
# These tests mock curl, openssl, jq, and base64 so no real network or crypto
# is required.
#
# The script hardcodes /workspace/github-token-expiry as its output path.
# Each test patches that path at runtime via sed before executing, redirecting
# writes to the per-test temp workspace directory.

SCRIPT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)/get-github-app-token.sh"

# ---------------------------------------------------------------------------
# setup / teardown
# ---------------------------------------------------------------------------
setup() {
  TMPDIR_TEST="$(mktemp -d)"
  WORKSPACE="${TMPDIR_TEST}/workspace"
  BIN_DIR="${TMPDIR_TEST}/bin"
  mkdir -p "$WORKSPACE" "$BIN_DIR"

  export GITHUB_APP_ID=12345
  export GITHUB_APP_INSTALLATION_ID=99999
  export GITHUB_APP_PRIVATE_KEY="FAKE_PRIVATE_KEY"

  # Mock openssl: emit fixed bytes so b64url has something to encode.
  cat > "${BIN_DIR}/openssl" <<'EOF'
#!/usr/bin/env bash
printf 'FAKESIG'
EOF
  chmod +x "${BIN_DIR}/openssl"

  # Mock jq: always return the fixed token string.
  cat > "${BIN_DIR}/jq" <<'EOF'
#!/usr/bin/env bash
printf 'ghs_abc123\n'
EOF
  chmod +x "${BIN_DIR}/jq"

  # Mock base64: consume stdin, emit a short fixed string.
  cat > "${BIN_DIR}/base64" <<'EOF'
#!/usr/bin/env bash
cat /dev/stdin | tr -d '\n' | od -A n -t x1 | tr -d ' \n' | head -c 32
printf '\n'
EOF
  chmod +x "${BIN_DIR}/base64"

  export PATH="${BIN_DIR}:${PATH}"
  export WORKSPACE
}

teardown() {
  rm -rf "$TMPDIR_TEST"
}

# ---------------------------------------------------------------------------
# Shared curl mock factories
# ---------------------------------------------------------------------------

_mock_curl_success() {
  cat > "${BIN_DIR}/curl" <<'EOF'
#!/usr/bin/env bash
printf '{"token":"ghs_abc123","expires_at":"2026-02-26T00:00:00Z"}\n'
EOF
  chmod +x "${BIN_DIR}/curl"
}

_mock_curl_fail() {
  cat > "${BIN_DIR}/curl" <<'EOF'
#!/usr/bin/env bash
exit 22
EOF
  chmod +x "${BIN_DIR}/curl"
}

# ---------------------------------------------------------------------------
# Helper: run the script with /workspace patched to $WORKSPACE.
# Usage: _run_patched_script
# Sets: $status, $output (via bats `run`)
# ---------------------------------------------------------------------------
_run_patched_script() {
  BEFORE=$(date +%s)
  run bash -c "
    export GITHUB_APP_ID='$GITHUB_APP_ID'
    export GITHUB_APP_INSTALLATION_ID='$GITHUB_APP_INSTALLATION_ID'
    export GITHUB_APP_PRIVATE_KEY='$GITHUB_APP_PRIVATE_KEY'
    export PATH='$PATH'
    bash <(sed 's|/workspace/github-token-expiry|${WORKSPACE}/github-token-expiry|g' '$SCRIPT')
  "
  AFTER=$(date +%s)
}

# ---------------------------------------------------------------------------
# Test 1: expiry file is written with correct integer value (NOW + 3500)
# ---------------------------------------------------------------------------
@test "expiry file is written with content equal to NOW+3500" {
  _mock_curl_success

  _run_patched_script

  [ "$status" -eq 0 ]

  [ -f "${WORKSPACE}/github-token-expiry" ]

  WRITTEN=$(cat "${WORKSPACE}/github-token-expiry")

  # Must be a plain integer.
  [[ "$WRITTEN" =~ ^[0-9]+$ ]]

  # Must be in the range [BEFORE+3500, AFTER+3500] (allows for test execution time).
  [ "$WRITTEN" -ge "$((BEFORE + 3500))" ]
  [ "$WRITTEN" -le "$((AFTER + 3500))" ]
}

# ---------------------------------------------------------------------------
# Test 2: stdout emits exactly the token string AND expiry file is written
# ---------------------------------------------------------------------------
@test "stdout emits exactly the token string and expiry file is written" {
  _mock_curl_success

  _run_patched_script

  [ "$status" -eq 0 ]

  # stdout must be exactly the token (with optional trailing newline stripped by bats).
  [ "$output" = "ghs_abc123" ]

  # Expiry file must also exist on a successful run.
  [ -f "${WORKSPACE}/github-token-expiry" ]
}

# ---------------------------------------------------------------------------
# Test 3: curl failure — exits non-zero, no expiry file, empty stdout
# ---------------------------------------------------------------------------
@test "when curl fails the script exits non-zero, no expiry file, empty stdout" {
  _mock_curl_fail

  _run_patched_script

  # Script must exit non-zero.
  [ "$status" -ne 0 ]

  # Expiry file must NOT exist.
  [ ! -f "${WORKSPACE}/github-token-expiry" ]

  # stdout must be completely empty — no partial token output.
  [ -z "$output" ]
}
