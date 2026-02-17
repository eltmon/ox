#!/usr/bin/env bash
# smoke-test-public-team-context.sh - Test public team context discovery via API
#
# Verifies that the CLI repo detail API returns the public team context
# (team_jihjpfkt8b) with correct visibility for the sageox/ox repo.
#
# Required environment variables (set by smoke-test.sh):
#   SAGEOX_ENDPOINT - API endpoint
#   SMOKE_TEST_WORKDIR - Temp directory for test
#   OX - Path to ox binary
#
# Public fixtures:
#   Repo: repo_019c5812-01e9-7b7d-b5b1-321c471c9777 (sageox/ox)
#   Team: team_jihjpfkt8b (sageox)

set -euo pipefail

# validate required env vars
: "${SAGEOX_ENDPOINT:?SAGEOX_ENDPOINT is required}"
: "${SMOKE_TEST_WORKDIR:?SMOKE_TEST_WORKDIR is required}"
: "${OX:?OX is required}"

EXPECTED_TEAM_ID="team_jihjpfkt8b"
REPO_ID="repo_019c5812-01e9-7b7d-b5b1-321c471c9777"

echo "Testing public team context discovery via API..."

# get auth token
AUTH_FILE="$HOME/.config/sageox/auth.json"
if [[ ! -f "$AUTH_FILE" ]]; then
    echo "error: no auth file found at $AUTH_FILE"
    exit 1
fi

# try endpoint-keyed token first, then fall back to first available
access_token=$(jq -r --arg ep "$SAGEOX_ENDPOINT" '.tokens[$ep].access_token // empty' "$AUTH_FILE")
if [[ -z "$access_token" ]]; then
    access_token=$(jq -r '[.tokens[]][0].access_token // empty' "$AUTH_FILE")
fi

if [[ -z "$access_token" ]]; then
    echo "error: no access token found for endpoint $SAGEOX_ENDPOINT"
    exit 1
fi

# fetch CLI repo detail API (note: /api/v1/cli/repos/, not /api/v1/repos/)
echo "Fetching repo detail for $REPO_ID..."
response=$(curl -s -w "\n%{http_code}" \
    -H "Authorization: Bearer $access_token" \
    "$SAGEOX_ENDPOINT/api/v1/cli/repos/$REPO_ID")

http_code=$(echo "$response" | tail -n1)
body=$(echo "$response" | sed '$d')

if [[ "$http_code" != "200" ]]; then
    echo "error: repo detail API returned status $http_code"
    echo "response: $body"
    exit 1
fi

# verify team contexts in response
tc_count=$(echo "$body" | jq '.team_contexts | length // 0')
echo "  team_contexts in response: $tc_count"

if [[ "$tc_count" -eq 0 ]]; then
    echo "error: no team contexts returned in repo detail"
    exit 1
fi

# check for expected public team
found_team=$(echo "$body" | jq -r --arg tid "$EXPECTED_TEAM_ID" \
    '.team_contexts[] | select(.team_id == $tid) | .team_id // empty')

if [[ -z "$found_team" ]]; then
    echo "error: expected team $EXPECTED_TEAM_ID not found in response"
    echo "  teams returned: $(echo "$body" | jq '[.team_contexts[].team_id]')"
    exit 1
fi
echo "  found expected team: $EXPECTED_TEAM_ID"

# verify team visibility is public
team_visibility=$(echo "$body" | jq -r --arg tid "$EXPECTED_TEAM_ID" \
    '.team_contexts[] | select(.team_id == $tid) | .visibility // empty')
echo "  team visibility: $team_visibility"

if [[ "$team_visibility" != "public" ]]; then
    echo "error: expected team visibility=public, got $team_visibility"
    exit 1
fi

# verify team has a clone URL
team_repo_url=$(echo "$body" | jq -r --arg tid "$EXPECTED_TEAM_ID" \
    '.team_contexts[] | select(.team_id == $tid) | .repo_url // empty')

if [[ -z "$team_repo_url" ]]; then
    echo "error: team context missing repo_url"
    exit 1
fi
echo "  team repo_url: present"

echo "Public team context discovery test passed"
exit 0
