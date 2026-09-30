package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sageox/ox/internal/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runHostContract(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(append([]string{"host-contract"}, args...))
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		_ = hostContractCmd.Flags().Set("json", "false")
	})
	require.NoError(t, rootCmd.Execute())
	return buf.String()
}

func TestHostContract_JSONHasTheFourKeys(t *testing.T) {
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(runHostContract(t, "--json")), &got))
	assert.Equal(t, "overdeck-host/1", got["contract"])
	assert.Equal(t, true, got["hostManaged"])
	assert.Equal(t, version.Version, got["version"])
	commit, ok := got["commit"].(string)
	assert.True(t, ok, "commit is a string (empty when the build has no VCS info)")
	assert.Equal(t, buildRevision(), commit)
	assert.Len(t, got, 4)
}

func TestHostContract_TextNamesTheContract(t *testing.T) {
	assert.Contains(t, runHostContract(t), "overdeck-host/1")
}

func TestHostContract_IsHidden(t *testing.T) {
	assert.True(t, hostContractCmd.Hidden, "host-contract is a host probe, not a user command")
}
