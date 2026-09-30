package telemetry

import (
	"testing"

	"github.com/sageox/ox/internal/config"
)

func TestEnabled_OffUnderHostManagedNetworkOff(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "")
	t.Setenv("SAGEOX_TELEMETRY", "")
	t.Setenv(config.EnvHostManaged, "1")
	t.Setenv(config.EnvHostNetwork, "")
	if Enabled() {
		t.Fatal("telemetry must be off under host-managed mode with the network off")
	}
	if NewClient("host-test").IsEnabled() {
		t.Fatal("a new telemetry client must start disabled offline")
	}
}
