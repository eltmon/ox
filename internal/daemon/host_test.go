package daemon

import (
	"testing"

	"github.com/sageox/ox/internal/config"
)

func TestHostOffline_DisablesDaemonTelemetryAndFriction(t *testing.T) {
	t.Setenv("SAGEOX_DAEMON", "")
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("SAGEOX_TELEMETRY", "")
	t.Setenv("SAGEOX_FRICTION", "")
	t.Setenv(config.EnvHostManaged, "1")
	t.Setenv(config.EnvHostNetwork, "off")
	if !IsDaemonDisabled() {
		t.Error("no daemon may start under host-managed mode with the network off")
	}
	if isTelemetryEnabled() {
		t.Error("daemon telemetry must be off offline")
	}
	if isFrictionEnabled() {
		t.Error("daemon friction must be off offline")
	}
}
