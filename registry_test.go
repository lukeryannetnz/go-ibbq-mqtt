package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestTrendSeriesIgnoresInvalidChannelsAndKeepsValidOnes(t *testing.T) {
	registry := &Registry{}
	if err := registry.Load(filepath.Join(t.TempDir(), "registry.json")); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	registry.RegisterDevice("AA:BB:CC:DD:EE:FF")
	if err := registry.SetName("AA:BB:CC:DD:EE:FF", "8D6E"); err != nil {
		t.Fatalf("SetName() error = %v", err)
	}

	registry.UpdateReadings("AA:BB:CC:DD:EE:FF", []float64{21.5, 0, 6553.5, 42.2}, -1)
	series := registry.TrendSeries()
	if len(series) != 2 {
		t.Fatalf("TrendSeries() len = %d, want 2", len(series))
	}
	if series[0].Name != "8D6E-1" || len(series[0].Points) != 1 || series[0].Points[0].Value != 21.5 {
		t.Fatalf("first series = %#v", series[0])
	}
	if series[1].Name != "8D6E-4" || len(series[1].Points) != 1 || series[1].Points[0].Value != 42.2 {
		t.Fatalf("second series = %#v", series[1])
	}
}

func TestTrimTrendPointsDropsOldData(t *testing.T) {
	now := time.Now().UTC()
	points := []TrendPoint{
		{Timestamp: now.Add(-13 * time.Hour), Value: 10},
		{Timestamp: now.Add(-11 * time.Hour), Value: 20},
	}
	trimmed := trimTrendPoints(points, now.Add(-12*time.Hour))
	if len(trimmed) != 1 || trimmed[0].Value != 20 {
		t.Fatalf("trimTrendPoints() = %#v, want only recent point", trimmed)
	}
}
