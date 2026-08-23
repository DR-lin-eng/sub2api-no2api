package config

import "testing"

func TestIPv6EgressRuntimeSwitchOverridesLegacyConfig(t *testing.T) {
	value := IPv6EgressConfig{Enabled: true}
	if !value.IsEnabled() {
		t.Fatal("legacy enabled value was not used before the runtime setting loaded")
	}
	value.SetRuntimeEnabled(false)
	if value.IsEnabled() {
		t.Fatal("runtime switch remained enabled after administrator disable")
	}
	value.SetRuntimeEnabled(true)
	if !value.IsEnabled() {
		t.Fatal("runtime switch did not re-enable")
	}
}
