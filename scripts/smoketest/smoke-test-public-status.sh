#!/usr/bin/env bash
# smoke-test-public-status.sh - Test ox status on the sageox/ox public repo
#
# Shallow-clones the real public repo and verifies that an authenticated
# user sees correct visibility and access info via ox status --json.
#
# Required environment variables (set by smoke-test.sh):
#   SAGEOX_ENDPOINT - API endpoint
#   SMOKE_TEST_WORKDIR - Temp directory for test
#   OX - Path to ox binary
#
# Public fixtures:
#   Repo:  repo_019c5812-01e9-7b7d-b5b1-321c471c9777  (sageox/ox)
#   Team:  team_jihjpfkt8b                               (sageox)

set -euo pipefail

# validate required env vars
: "${SAGEOX_ENDPOINT:?SAGEOX_ENDPOINT is required}"
: "${SMOKE_TEST_WORKDIR:?SMOKE_TEST_WORKDIR is required}"
: "${OX:?OX is required}"

PUBLIC_REPO_URL="https://github.com/sageox/ox.git"
TEST_REPO="$SMOKE_TEST_WORKDIR/test-public-ox"

echo "Testing ox status on public repo (sageox/ox)..."

# shallow clone the real public repo (has .sageox/ with correct repo ID)
if [[ ! -d "$TEST_REPO" ]]; then
    echo "Shallow cloning $PUBLIC_REPO_URL..."
    git clone --depth 1 -q "$PUBLIC_REPO_URL" "$TEST_REPO"
fi

cd "$TEST_REPO"

# verify .sageox/ exists from the clone
if [[ ! -d ".sageox" ]]; then
    echo "error: cloned repo missing .sageox/ directory"
    exit 1
fi

# bootstrap config.local.toml so ox status populates the ledger section.
# the cloned repo has config.json (repo_id, endpoint) but config.local.toml
# is gitignored and only created by the daemon. we need a ledger path entry
# so buildStatusJSON includes visibility/access from the API.
if [[ ! -f ".sageox/config.local.toml" ]]; then
    repo_name=$(basename "$TEST_REPO")
    parent_dir=$(dirname "$TEST_REPO")
    ledger_path="$parent_dir/${repo_name}_sageox/sageox.ai/ledger"
    echo "Bootstrapping config.local.toml (ledger_path=$ledger_path)"
    cat > .sageox/config.local.toml << EOF
[ledger]
path = "$ledger_path"
EOF
fi

echo "Running ox status --json..."
set +e
status_output=$($OX status --json 2>&1)
exit_code=$?
set -e

if [[ $exit_code -ne 0 ]]; then
    echo "error: ox status --json failed with exit code $exit_code"
    echo "output: $status_output"
    exit 1
fi

# validate JSON is parseable
if ! echo "$status_output" | jq . >/dev/null 2>&1; then
    echo "error: ox status --json did not produce valid JSON"
    echo "output: $status_output"
    exit 1
fi

echo "JSON output received, validating fields..."

# check visibility
visibility=$(echo "$status_output" | jq -r '.ledger.visibility // empty')
if [[ -z "$visibility" ]]; then
    echo "error: .ledger.visibility is missing or empty"
    echo "ledger section: $(echo "$status_output" | jq '.ledger')"
    exit 1
fi

if [[ "$visibility" != "public" ]]; then
    echo "error: expected visibility=public, got visibility=$visibility"
    exit 1
fi
echo "  visibility: $visibility"

# check access level
access_level=$(echo "$status_output" | jq -r '.ledger.access_level // empty')
if [[ -z "$access_level" ]]; then
    echo "error: .ledger.access_level is missing or empty"
    exit 1
fi

if [[ "$access_level" != "member" && "$access_level" != "viewer" ]]; then
    echo "error: expected access_level=member or viewer, got access_level=$access_level"
    exit 1
fi
echo "  access_level: $access_level"

# check team contexts are present
tc_count=$(echo "$status_output" | jq '.team_contexts | length // 0')
if [[ "$tc_count" -eq 0 ]]; then
    echo "warning: no team contexts found (may be expected for some configurations)"
else
    echo "  team_contexts: $tc_count found"
fi

echo "Public repo status test passed"
exit 0
