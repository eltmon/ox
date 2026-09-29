package config

import (
	"errors"
	"net/http"
	"os"
	"strings"
)

// Host-managed mode (Overdeck, eltmon/ox fork). A host that launches coding
// agents sets OX_HOST_MANAGED=1 and supplies the hooks and env itself, so ox
// must not write into the repository (prime markers, agent hook files, skill
// inventories), must not add attribution, and must not tell the agent to run
// ox init or ox login. Unset, ox behaves exactly like upstream.
const (
	// EnvHostManaged switches on host-managed mode.
	// Consumed by: HostManaged()
	EnvHostManaged = "OX_HOST_MANAGED"

	// EnvHostNetwork is the host network gate: "on" allows network calls
	// under host-managed mode; unset or any other value means off.
	// Consumed by: HostNetworkAllowed()
	EnvHostNetwork = "OX_HOST_NETWORK"
)

// HostManaged reports whether OX_HOST_MANAGED is set to 1, true, yes or on
// (case-insensitive).
func HostManaged() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvHostManaged))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// HostNetworkAllowed reports whether OX_HOST_NETWORK is "on" (case-insensitive).
func HostNetworkAllowed() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv(EnvHostNetwork)), "on")
}

// HostOffline reports whether host-managed mode keeps ox off the network:
// OX_HOST_MANAGED is set and OX_HOST_NETWORK is not "on". Offline, session
// publishing is manual, telemetry, friction, GitHub sync, cloud query and
// OpenTelemetry export are off, no daemon starts, and the network guard
// refuses every HTTP request that goes through http.DefaultTransport.
func HostOffline() bool {
	return HostManaged() && !HostNetworkAllowed()
}

// ErrHostNetworkOff is returned for every request the network guard refuses.
var ErrHostNetworkOff = errors.New("network is off: " + EnvHostManaged + "=1 without " + EnvHostNetwork + "=on")

type refusingTransport struct{}

func (refusingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, ErrHostNetworkOff
}

// InstallHostNetworkGuard replaces http.DefaultTransport with a transport
// that refuses every request when HostOffline() is true, and is a no-op
// otherwise. Clients that leave Transport nil (most ox clients) go through
// http.DefaultTransport and so through the guard. It returns a function that
// restores the previous transport.
func InstallHostNetworkGuard() (restore func()) {
	if !HostOffline() {
		return func() {}
	}
	prev := http.DefaultTransport
	http.DefaultTransport = refusingTransport{}
	return func() { http.DefaultTransport = prev }
}
