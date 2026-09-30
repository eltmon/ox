//go:build !short

package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageox/ox/internal/config"
	"github.com/sageox/ox/internal/ledger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withPrimeStdin feeds prime the hook payload a SessionStart hook would pipe
// to it, so prime writes the session marker the later hooks look up.
func withPrimeStdin(t *testing.T, payload string) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(payload)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// runSessionLifecycle drives prime and the hook events of one short session.
func runSessionLifecycle(t *testing.T, sessionID string) {
	t.Helper()
	t.Setenv("AGENT_ENV", "claude-code")
	withPrimeStdin(t, `{"session_id":"`+sessionID+`","hook_event_name":"SessionStart","source":"startup"}`)
	_, err := runPrimeCaptured(t, "")
	require.NoError(t, err)

	withHookInput(t, sessionID)
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop"} {
		_, err := captureHostStdout(t, func() error { return runAgentHook([]string{event}) })
		require.NoErrorf(t, err, "hook %s", event)
	}
}

// provisionLedger clones a local bare repo into the default ledger path, the
// way the daemon does once with the network on. Recording needs this clone;
// it is not a network operation afterwards.
func provisionLedger(t *testing.T, repo string) string {
	t.Helper()
	bare, _ := createBareAndClone(t)
	path, err := ledger.DefaultPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	runGit(t, repo, "clone", "--quiet", bare, path)
	require.True(t, ledger.Exists(""))
	return path
}

// FR-12 / CP-1: logged in, with an endpoint that would answer, host-managed
// mode with the network off sends nothing, even with OX_SESSION_PUBLISHING=auto,
// and still records the session into the local ledger cache.
//
// Upstream's network work in this lifecycle is daemon-driven or asynchronous,
// so even with the network on this harness sees no request from it; the
// guard's own refusal (asserted directly below and in internal/config) is the
// load-bearing evidence that nothing through http.DefaultTransport can dial.
func TestHostOffline_PrimeAndHooksSendNothingAndRecordLocally(t *testing.T) {
	env := initializedE2E(t)
	ledgerPath := provisionLedger(t, env.Root)
	before := len(env.Requested())

	t.Setenv(config.EnvHostManaged, "1")
	t.Setenv(config.EnvHostNetwork, "")
	t.Setenv(config.EnvSessionPublishing, config.SessionPublishingAuto)
	t.Cleanup(config.InstallHostNetworkGuard())

	require.Equal(t, config.SessionPublishingManual, config.ResolveSessionPublishing(env.Root).Mode)
	runSessionLifecycle(t, "host-offline-session")
	_, err := captureHostStdout(t, func() error { return runAgentHook([]string{"SessionEnd"}) })
	require.NoError(t, err)

	_, err = http.Get(env.Server.URL + "/api/v1/auth/introspect") //nolint:noctx // test
	require.ErrorIs(t, err, config.ErrHostNetworkOff, "the guard is active in this process")
	assert.Empty(t, env.Requested()[before:], "the SageOx endpoint must see no requests while the network is off")

	sessions, err := filepath.Glob(filepath.Join(ledgerPath, ".sageox", "cache", "sessions", "*", "raw.jsonl"))
	require.NoError(t, err)
	assert.NotEmpty(t, sessions, "CP-1: offline host-managed mode records the session locally")
}
