package config

import (
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
