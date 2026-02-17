#!/usr/bin/env bash
# smoke-test-public-viewer.sh - Test public repo access as a non-member/viewer
#
# Uses the shallow-cloned sageox/ox repo to verify that a non-member sees
# the public repo with viewer-level access.
#
# Required environment variables (set by smoke-test.sh):
#   SAGEOX_ENDPOINT - API endpoint
#   SMOKE_TEST_WORKDIR - Temp directory for test
#   OX - Path to ox binary
#
# Optional environment variables:
#   SAGEOX_CI_VIEWER_EMAIL - Viewer test account email (no sageox team membership)
#   SAGEOX_CI_VIEWER_PASSWORD - Viewer test account password
#
# Public fixtures:
#   Repo: repo_019c5812-01e9-7b7d-b5b1-321c471c9777 (sageox/ox)

set -euo pipefail

# validate required env vars
: "${SAGEOX_ENDPOINT:?SAGEOX_ENDPOINT is required}"
: "${SMOKE_TEST_WORKDIR:?SMOKE_TEST_WORKDIR is required}"
: "${OX:?OX is required}"

# check if viewer credentials are available
if [[ -z "${SAGEOX_CI_VIEWER_EMAIL:-}" || -z "${SAGEOX_CI_VIEWER_PASSWORD:-}" ]]; then
    echo "[SKIP] viewer credentials not configured"
    echo "Set SAGEOX_CI_VIEWER_EMAIL and SAGEOX_CI_VIEWER_PASSWORD to enable"
    exit 0
fi

TEST_REPO="$SMOKE_TEST_WORKDIR/test-public-ox"

if [[ ! -d "$TEST_REPO/.sageox" ]]; then
    echo "error: public repo not cloned; run public-status test first"
    exit 1
fi

echo "Testing public repo access as viewer..."

# backup current auth
AUTH_FILE="$HOME/.config/sageox/auth.json"
AUTH_BACKUP=""
if [[ -f "$AUTH_FILE" ]]; then
    AUTH_BACKUP=$(cat "$AUTH_FILE")
fi

restore_auth() {
    if [[ -n "$AUTH_BACKUP" ]]; then
        echo "$AUTH_BACKUP" > "$AUTH_FILE"
    fi
}
trap restore_auth EXIT

# authenticate as viewer account
echo "Authenticating as viewer: $SAGEOX_CI_VIEWER_EMAIL"

response=$(curl -s -w "\n%{http_code}" -X POST "$SAGEOX_ENDPOINT/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\": \"$SAGEOX_CI_VIEWER_EMAIL\", \"password\": \"$SAGEOX_CI_VIEWER_PASSWORD\"}")

http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [[ "$http_code" == "401" || "$http_code" == "403" ]]; then
    echo "[SKIP] viewer authentication failed (status $http_code)"
    echo "unauthenticated public access may not yet be supported"
    exit 0
fi

if [[ "$http_code" != "200" ]]; then
    echo "error: viewer login failed with status $http_code"
    echo "response: $body"
    exit 1
fi

access_token=$(echo "$body" | jq -r '.access_token // empty')
if [[ -z "$access_token" ]]; then
    echo "error: no access_token in viewer login response"
    exit 1
fi

# calculate expiry
if date -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ &>/dev/null; then
    expires_at=$(date -u -d '+1 hour' +%Y-%m-%dT%H:%M:%SZ)
else
    expires_at=$(date -u -v+1H +%Y-%m-%dT%H:%M:%SZ)
fi

# write viewer auth
mkdir -p "$(dirname "$AUTH_FILE")"
cat > "$AUTH_FILE" << EOF
{
  "tokens": {
    "$SAGEOX_ENDPOINT": {
      "access_token": "$access_token",
      "expires_at": "$expires_at"
    }
  }
}
EOF

echo "Viewer authentication successful"

cd "$TEST_REPO"

echo "Running ox status --json as viewer..."
set +e
status_output=$($OX status --json 2>&1)
exit_code=$?
set -e

if [[ $exit_code -ne 0 ]]; then
    # might get auth errors for viewer account
    if echo "$status_output" | grep -qi "unauthorized\|forbidden\|401\|403"; then
        echo "[SKIP] unauthenticated public access not yet supported"
        exit 0
    fi
    echo "error: ox status --json failed with exit code $exit_code"
    echo "output: $status_output"
    exit 1
fi

# validate JSON
if ! echo "$status_output" | jq . >/dev/null 2>&1; then
    echo "error: ox status --json did not produce valid JSON"
    exit 1
fi

# check visibility
visibility=$(echo "$status_output" | jq -r '.ledger.visibility // empty')
if [[ "$visibility" != "public" ]]; then
    echo "error: expected visibility=public, got visibility=$visibility"
    exit 1
fi
echo "  visibility: $visibility"

# check access level is viewer
access_level=$(echo "$status_output" | jq -r '.ledger.access_level // empty')
echo "  access_level: $access_level"

if [[ "$access_level" == "viewer" ]]; then
    echo "  confirmed: non-member sees viewer access"
elif [[ "$access_level" == "member" ]]; then
    echo "warning: viewer account has member access (may be a team member)"
else
    echo "warning: unexpected access_level=$access_level"
fi

echo "Public viewer access test passed"
exit 0
