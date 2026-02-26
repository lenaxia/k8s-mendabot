#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_APP_ID:?GITHUB_APP_ID must be set}"
: "${GITHUB_APP_INSTALLATION_ID:?GITHUB_APP_INSTALLATION_ID must be set}"
: "${GITHUB_APP_PRIVATE_KEY:?GITHUB_APP_PRIVATE_KEY must be set}"

NOW=$(date +%s)
IAT=$((NOW - 60))
EXP=$((NOW + 540))

b64url() { base64 -w0 | tr '+/' '-_' | tr -d '='; }

HEADER=$(printf '{"alg":"RS256","typ":"JWT"}' | b64url)
# iss must be a JSON number (integer), not a string — GitHub rejects string iss values.
PAYLOAD=$(printf '{"iat":%d,"exp":%d,"iss":%d}' "$IAT" "$EXP" "$GITHUB_APP_ID" | b64url)
UNSIGNED="${HEADER}.${PAYLOAD}"

# Write the private key to a temp file to avoid process substitution (<(…)) which
# requires /dev/fd and may not be available in hardened container environments.
KEY_FILE=$(mktemp)
trap 'rm -f "$KEY_FILE"' EXIT
printf '%s' "$GITHUB_APP_PRIVATE_KEY" > "$KEY_FILE"

SIGNATURE=$(printf '%s' "$UNSIGNED" \
  | openssl dgst -sha256 -sign "$KEY_FILE" \
  | b64url)

JWT="${UNSIGNED}.${SIGNATURE}"

RESPONSE=$(curl -sf \
  -X POST \
  -H "Authorization: Bearer ${JWT}" \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "https://api.github.com/app/installations/${GITHUB_APP_INSTALLATION_ID}/access_tokens")

# Write expiry timestamp: NOW (captured at JWT mint time) + 3500 s.
# GitHub installation tokens are valid for 3600 s; the 3500 s window gives the
# main container a 100 s head-start before the pre-flight guard triggers.
printf '%d' "$((NOW + 3500))" > /workspace/github-token-expiry

printf '%s\n' "$(printf '%s' "$RESPONSE" | jq -r '.token')"
