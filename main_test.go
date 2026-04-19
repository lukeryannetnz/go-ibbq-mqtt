package main

import (
	"os"
	"reflect"
	"testing"
)

func TestReadDeviceConfigsLegacy(t *testing.T) {
	t.Setenv("DEVICE_NAMES", "")
	t.Setenv("DEVICE_MACS", "")
	t.Setenv("DEVICE_NAME", "probe1")
	t.Setenv("DEVICE_MAC", "AA:BB:CC:DD:EE:FF")

	got := readDeviceConfigs()
	want := []deviceConfig{{
		name: "probe1",
		mac:  "AA:BB:CC:DD:EE:FF",
	}}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readDeviceConfigs() = %#v, want %#v", got, want)
	}
}

func TestReadDeviceConfigsMulti(t *testing.T) {
	t.Setenv("DEVICE_NAMES", "probe1, probe2")
	t.Setenv("DEVICE_MACS", "AA:BB:CC:DD:EE:FF,11:22:33:44:55:66")
	t.Setenv("DEVICE_NAME", "")
	t.Setenv("DEVICE_MAC", "")

	got := readDeviceConfigs()
	want := []deviceConfig{
		{name: "probe1", mac: "AA:BB:CC:DD:EE:FF"},
		{name: "probe2", mac: "11:22:33:44:55:66"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readDeviceConfigs() = %#v, want %#v", got, want)
	}
}

func TestReadDeviceConfigsPadsMissingNames(t *testing.T) {
	t.Setenv("DEVICE_NAMES", "probe1")
	t.Setenv("DEVICE_MACS", "AA:BB:CC:DD:EE:FF,11:22:33:44:55:66")

	got := readDeviceConfigs()
	want := []deviceConfig{
		{name: "probe1", mac: "AA:BB:CC:DD:EE:FF"},
		{name: "device2", mac: "11:22:33:44:55:66"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("readDeviceConfigs() = %#v, want %#v", got, want)
	}
}

func TestSplitTrimmed(t *testing.T) {
	got := splitTrimmed(" probe1, , probe2 ,,probe3 ")
	want := []string{"probe1", "probe2", "probe3"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitTrimmed() = %#v, want %#v", got, want)
	}
}

func TestMainEnvAvailable(t *testing.T) {
	if os.Getenv("MQTT_TOPIC") == "" {
		t.Fatal("expected MQTT_TOPIC to be set by test setup")
	}
}
