package config

import "testing"

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
