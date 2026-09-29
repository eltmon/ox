package config

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHostManaged(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"off", false},
		{"banana", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{" yes ", true},
		{"On", true},
	} {
		t.Setenv(EnvHostManaged, tc.value)
		if got := HostManaged(); got != tc.want {
			t.Errorf("OX_HOST_MANAGED=%q: HostManaged() = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestHostNetworkGate(t *testing.T) {
	for _, tc := range []struct {
		managed, network string
		allowed, offline bool
	}{
		{"", "", false, false},
		{"", "on", true, false},
		{"1", "", false, true},
		{"1", "off", false, true},
		{"1", "banana", false, true},
		{"1", "ON", true, false},
		{"1", " on ", true, false},
	} {
		t.Setenv(EnvHostManaged, tc.managed)
		t.Setenv(EnvHostNetwork, tc.network)
		if got := HostNetworkAllowed(); got != tc.allowed {
			t.Errorf("managed=%q network=%q: HostNetworkAllowed() = %v, want %v", tc.managed, tc.network, got, tc.allowed)
		}
		if got := HostOffline(); got != tc.offline {
			t.Errorf("managed=%q network=%q: HostOffline() = %v, want %v", tc.managed, tc.network, got, tc.offline)
		}
	}
}

func TestInstallHostNetworkGuard_RefusesEveryRequestOffline(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Setenv(EnvHostManaged, "1")
	t.Setenv(EnvHostNetwork, "")
	restore := InstallHostNetworkGuard()
	_, err := http.Get(server.URL) //nolint:noctx // test
	restore()
	if !errors.Is(err, ErrHostNetworkOff) {
		t.Fatalf("offline GET error = %v, want ErrHostNetworkOff", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("server saw %d requests through the guard, want 0", hits.Load())
	}

	// restored: the same request goes through
	resp, err := http.Get(server.URL) //nolint:noctx // test
	if err != nil {
		t.Fatalf("GET after restore: %v", err)
	}
	_ = resp.Body.Close()
	if hits.Load() != 1 {
		t.Fatalf("server saw %d requests after restore, want 1", hits.Load())
	}
}

func TestInstallHostNetworkGuard_NoOpUnlessOffline(t *testing.T) {
	before := http.DefaultTransport
	for _, env := range [][2]string{{"", ""}, {"1", "on"}} {
		t.Setenv(EnvHostManaged, env[0])
		t.Setenv(EnvHostNetwork, env[1])
		InstallHostNetworkGuard()()
		if http.DefaultTransport != before {
			t.Fatalf("managed=%q network=%q: guard replaced the transport", env[0], env[1])
		}
	}
}

func TestHostOffline_ForcesManualPublishingAndOffSwitches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(EnvSessionPublishing, SessionPublishingAuto)
	t.Setenv(EnvGitHubSync, GitHubSyncEnabled)

	t.Setenv(EnvHostManaged, "1")
	t.Setenv(EnvHostNetwork, "")
	got := ResolveSessionPublishing("")
	if got.Mode != SessionPublishingManual || got.Source != SessionRecordingSourceHost {
		t.Fatalf("offline publishing = %+v, want manual from host", got)
	}
	if ResolveGitHubSync("") != GitHubSyncDisabled || ResolveGitHubSyncPRs("") != GitHubSyncDisabled || ResolveGitHubSyncIssues("") != GitHubSyncDisabled {
		t.Fatal("offline GitHub sync must resolve disabled")
	}
	if ResolveUserPromptSubmitCloudQuery("") {
		t.Fatal("offline cloud query must resolve off")
	}

	// uploads on: the env override applies again
	t.Setenv(EnvHostNetwork, "on")
	got = ResolveSessionPublishing("")
	if got.Mode != SessionPublishingAuto || got.Source != SessionRecordingSourceEnv {
		t.Fatalf("network-on publishing = %+v, want auto from env", got)
	}
	if ResolveGitHubSync("") != GitHubSyncEnabled {
		t.Fatal("network-on GitHub sync follows the env")
	}
}
