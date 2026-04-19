package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterDevicePersistsAndLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	registry := &Registry{}
	if err := registry.Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	record := registry.RegisterDevice("aa:bb:cc:dd:ee:ff")
	if record == nil {
		t.Fatal("RegisterDevice() returned nil")
	}
	if record.Config.Name != "EE:FF" {
		t.Fatalf("default name = %q, want EE:FF", record.Config.Name)
	}

	reloaded := &Registry{}
	if err := reloaded.Load(path); err != nil {
		t.Fatalf("reloaded Load() error = %v", err)
	}
	got := reloaded.Get("AA:BB:CC:DD:EE:FF")
	if got == nil {
		t.Fatal("Get() returned nil after reload")
	}
	if got.Config.UID == "" {
		t.Fatal("expected UID to persist")
	}
	if got.Config.Name != "EE:FF" {
		t.Fatalf("reloaded name = %q, want EE:FF", got.Config.Name)
	}
}

func TestSetConfigAndPollValidation(t *testing.T) {
	registry := &Registry{}
	if err := registry.Load(filepath.Join(t.TempDir(), "registry.json")); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	registry.RegisterDevice("AA:BB:CC:DD:EE:FF")

	if err := registry.SetPollInterval("AA:BB:CC:DD:EE:FF", 0); err == nil {
		t.Fatal("expected SetPollInterval to reject zero")
	}

	if err := registry.SetConfig(AppConfig{ScanIntervalSec: 0, DefaultPollIntervalSec: 0}); err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}
	cfg := registry.Config()
	if cfg.ScanIntervalSec != defaultScanIntervalSec || cfg.DefaultPollIntervalSec != defaultPollIntervalSec {
		t.Fatalf("Config() = %#v, want defaults", cfg)
	}
}

func TestConfigureEnvUsesEnvironmentWithoutDotEnv(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	t.Setenv("MQTT_SERVER", "tcp://localhost:1883")
	configureEnv()
}
