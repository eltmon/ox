//go:build !short

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sageox/agentx"
	"github.com/sageox/ox/internal/auth"
	"github.com/sageox/ox/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Host-managed mode (OX_HOST_MANAGED=1, eltmon/ox fork for Overdeck): the host
// supplies hooks and env, so prime and hooks must leave the repository exactly
// as they found it, and must never tell the agent to run ox init or ox login.

// hostCommitAll commits every change in repo so a later `git status --porcelain`
// shows only what the command under test wrote.
func hostCommitAll(t *testing.T, repo string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "--allow-empty", "-m", "baseline"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, out)
	}
}

func hostPorcelain(t *testing.T, repo string) string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all")
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git status: %s", out)
	return strings.TrimSpace(string(out))
}

// stripRepoIntegration removes what `ox init` wrote outside .sageox/ (the
// ox:prime markers and the Claude hook file), so upstream prime's anti-entropy
// would have something to put back.
func stripRepoIntegration(t *testing.T, repo string) {
	t.Helper()
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		path := filepath.Join(repo, name)
		if _, err := os.Stat(path); err == nil {
			require.NoError(t, os.WriteFile(path, []byte("# project notes\n"), 0o644))
		}
	}
	require.NoError(t, os.RemoveAll(filepath.Join(repo, ".claude")))
	hostCommitAll(t, repo)
}

func runPrimeCaptured(t *testing.T, format string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd := agentPrimeCmd
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	require.NoError(t, cmd.Flags().Set("agent", "claude-code"))
	require.NoError(t, cmd.Flags().Set("format", format))
	t.Cleanup(func() {
		_ = cmd.Flags().Set("agent", "")
		_ = cmd.Flags().Set("format", "")
		cmd.SetOut(nil)
		cmd.SetErr(nil)
	})
	err := runAgentPrime(cmd, nil)
	return buf.String(), err
}

// captureHostStdout runs fn with os.Stdout redirected, because hook handlers write
// straight to the process stdout (that is what reaches the agent's context).
func captureHostStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	runErr := fn()
	os.Stdout = orig
	require.NoError(t, w.Close())
	return <-done, runErr
}

func withHookInput(t *testing.T, sessionID string) {
	t.Helper()
	orig := ReadHookInput
	ReadHookInput = func() *agentx.HookInput {
		return &agentx.HookInput{SessionID: sessionID, RawBytes: []byte(`{"session_id":"` + sessionID + `"}`)}
	}
	t.Cleanup(func() { ReadHookInput = orig })
}

func TestHostManaged_PrimeLeavesTheRepoUntouched(t *testing.T) {
	env := initializedE2E(t)
	stripRepoIntegration(t, env.Root)
	t.Setenv(config.EnvHostManaged, "1")

	out, err := runPrimeCaptured(t, "")
	require.NoError(t, err)
	require.NotEmpty(t, out, "an initialized repo still primes under host-managed mode")
	assert.Empty(t, hostPorcelain(t, env.Root), "host-managed prime must not write into the repository")
}

// Control for the test above: without host-managed mode, upstream prime's
// anti-entropy restores what was stripped, so the untouched assertion is not
// vacuous.
func TestHostManaged_UnsetKeepsUpstreamAntiEntropy(t *testing.T) {
	env := initializedE2E(t)
	stripRepoIntegration(t, env.Root)
	t.Setenv(config.EnvHostManaged, "")

	_, err := runPrimeCaptured(t, "")
	require.NoError(t, err)
	assert.NotEmpty(t, hostPorcelain(t, env.Root), "upstream prime re-adds its repo integration")
}

func TestHostManaged_HooksLeaveTheRepoUntouched(t *testing.T) {
	env := initializedE2E(t)
	stripRepoIntegration(t, env.Root)
	t.Setenv(config.EnvHostManaged, "1")
	t.Setenv("AGENT_ENV", "claude-code")
	withHookInput(t, "host-managed-hook-session")

	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PostToolUse", "Stop", "SessionEnd"} {
		_, err := captureHostStdout(t, func() error { return runAgentHook([]string{event}) })
		require.NoErrorf(t, err, "hook %s", event)
	}
	assert.Empty(t, hostPorcelain(t, env.Root), "host-managed hooks must not write into the repository")
}

func TestHostManaged_UninitializedRepoIsSilent(t *testing.T) {
	env := newOxE2E(t)
	t.Setenv(config.EnvHostManaged, "1")

	out, err := runPrimeCaptured(t, "")
	require.NoError(t, err, "prime exits 0 in an uninitialized repo")
	assert.Empty(t, out, "no 'run ox init' instruction may reach the agent")

	withHookInput(t, "host-managed-uninit-session")
	hookOut, err := captureHostStdout(t, func() error { return runAgentHook([]string{"SessionStart"}) })
	require.NoError(t, err)
	assert.Empty(t, hookOut)
	assert.Empty(t, hostPorcelain(t, env.Root))
}

func TestHostManaged_LoggedOutPrimeHasNoLoginInstruction(t *testing.T) {
	env := initializedE2E(t)
	require.NoError(t, auth.RemoveTokenForEndpoint(env.Server.URL))
	t.Setenv("FEATURE_AUTH", "true") // the degraded (logged-out) prime path is behind this flag
	t.Setenv(config.EnvHostManaged, "1")

	out, err := runPrimeCaptured(t, "json")
	require.NoError(t, err)
	assert.Contains(t, out, "degraded", "logged out, prime reports degraded mode")
	assert.NotContains(t, out, "ox login")
	assert.NotContains(t, out, "Not logged in")
}

// Control for the test above: upstream's logged-out prime does ask for ox login.
func TestHostManaged_UnsetLoggedOutPrimeAsksForLogin(t *testing.T) {
	env := initializedE2E(t)
	require.NoError(t, auth.RemoveTokenForEndpoint(env.Server.URL))
	t.Setenv("FEATURE_AUTH", "true")
	t.Setenv(config.EnvHostManaged, "")

	out, err := runPrimeCaptured(t, "json")
	require.NoError(t, err)
	assert.Contains(t, out, "ox login")
}

// attributionMarkers are strings that only appear when prime tells the agent
// to credit SageOx (guidance, plan footer, commit/PR trailers).
var attributionMarkers = []string{"Co-Authored-By", "Guided by SageOx", "<attribution>", "SageOx Attribution", "Attribution is"}

func TestHostManaged_PrimeCarriesNoAttribution(t *testing.T) {
	initializedE2E(t)
	t.Setenv(config.EnvHostManaged, "1")

	for _, format := range []string{"xml", "json"} {
		out, err := runPrimeCaptured(t, format)
		require.NoError(t, err)
		require.NotEmpty(t, out)
		for _, marker := range attributionMarkers {
			assert.NotContainsf(t, out, marker, "host-managed %s prime must carry no attribution", format)
		}
	}
	assert.Equal(t, config.ResolvedAttribution{}, loadResolvedAttribution(), "every attribution value resolves empty")
}

// Control: upstream prime does tell the agent how to attribute.
func TestHostManaged_UnsetPrimeCarriesAttribution(t *testing.T) {
	initializedE2E(t)
	t.Setenv(config.EnvHostManaged, "")

	for _, format := range []string{"xml", "json"} {
		out, err := runPrimeCaptured(t, format)
		require.NoError(t, err)
		assert.Containsf(t, out, "Co-Authored-By", "upstream %s prime carries commit attribution", format)
		assert.Containsf(t, out, "Guided by SageOx", "upstream %s prime carries the plan footer", format)
	}
}

// changedPaths lists every path git reports as changed or untracked.
func changedPaths(t *testing.T, repo string) []string {
	t.Helper()
	var paths []string
	for _, line := range strings.Split(hostPorcelain(t, repo), "\n") {
		if len(line) > 3 {
			paths = append(paths, strings.TrimSpace(line[3:]))
		}
	}
	return paths
}

func gitHookNames(t *testing.T, repo string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(repo, ".git", "hooks"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	var names []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sample") {
			names = append(names, e.Name())
		}
	}
	return names
}

func runInitWithClaude(t *testing.T, env *oxE2E) {
	t.Helper()
	withInitFlags(t, env.TeamID)
	initAgentsFlag = "claude-code"
	require.NoError(t, runInit())
}

func TestHostManaged_InitWritesOnlySageox(t *testing.T) {
	env := newOxE2E(t)
	t.Setenv(config.EnvHostManaged, "1")
	t.Setenv(config.EnvHostNetwork, "on") // init registers the repo with the API
	hooksBefore := gitHookNames(t, env.Root)

	runInitWithClaude(t, env)

	require.True(t, config.IsInitialized(env.Root), "host-managed init still initializes .sageox/")
	for _, path := range changedPaths(t, env.Root) {
		assert.Truef(t, strings.HasPrefix(path, ".sageox/"), "host-managed init wrote %s outside .sageox/", path)
	}
	assert.Equal(t, hooksBefore, gitHookNames(t, env.Root), "host-managed init installs no git hooks")
}

// Control: upstream init with Claude Code selected writes outside .sageox/.
func TestHostManaged_UnsetInitWritesAgentIntegration(t *testing.T) {
	env := newOxE2E(t)
	t.Setenv(config.EnvHostManaged, "")

	runInitWithClaude(t, env)

	var outside []string
	for _, path := range changedPaths(t, env.Root) {
		if !strings.HasPrefix(path, ".sageox/") {
			outside = append(outside, path)
		}
	}
	assert.NotEmpty(t, outside, "upstream init writes AI coworker integration files")
}
