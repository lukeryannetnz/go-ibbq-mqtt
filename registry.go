package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultScanIntervalSec        = 300
	defaultPollIntervalSec        = 5
	defaultDeadAfter              = 10 * time.Minute
	defaultDeadCheckerInterval    = 30 * time.Second
	defaultDiscoveryScanDuration  = 30 * time.Second
	defaultWebRefreshIntervalSecs = 5
	trendRetention               = 12 * time.Hour
)

type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type TrendSeries struct {
	Name   string       `json:"name"`
	Points []TrendPoint `json:"points"`
}

type MQTTPublishRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Topic     string    `json:"topic"`
	Payload   string    `json:"payload"`
}

// DeviceConfig is the persisted part of a device record.
type DeviceConfig struct {
	MAC             string    `json:"mac"`
	UID             string    `json:"uid"`
	Name            string    `json:"name"`
	PollIntervalSec int       `json:"pollIntervalSec"`
	FirstSeen       time.Time `json:"firstSeen"`
}

// DeviceRecord combines persisted config with runtime state.
type DeviceRecord struct {
	Config        DeviceConfig
	Status        string
	LastSeen      time.Time
	Temperatures  []float64
	BatteryLevel  int
	lastPublished time.Time
	sensorHistory map[int][]TrendPoint
}

// AppConfig is the persisted global configuration.
type AppConfig struct {
	ScanIntervalSec        int `json:"scanIntervalSec"`
	DefaultPollIntervalSec int `json:"defaultPollIntervalSec"`
}

// RegistryFile is the structure written to disk.
type RegistryFile struct {
	Config  AppConfig               `json:"config"`
	Devices map[string]DeviceConfig `json:"devices"`
}

// Registry is the in-memory store.
type Registry struct {
	mu              sync.RWMutex
	config          AppConfig
	devices         map[string]*DeviceRecord
	path            string
	mqttConnected   bool
	recentPublishes []MQTTPublishRecord
}

func defaultAppConfig() AppConfig {
	return AppConfig{
		ScanIntervalSec:        defaultScanIntervalSec,
		DefaultPollIntervalSec: defaultPollIntervalSec,
	}
}

func normalizeAppConfig(cfg AppConfig) AppConfig {
	if cfg.ScanIntervalSec <= 0 {
		cfg.ScanIntervalSec = defaultScanIntervalSec
	}
	if cfg.DefaultPollIntervalSec <= 0 {
		cfg.DefaultPollIntervalSec = defaultPollIntervalSec
	}
	return cfg
}

func normalizeMAC(mac string) string {
	return strings.ToUpper(strings.TrimSpace(mac))
}

func defaultDeviceName(mac string) string {
	mac = normalizeMAC(mac)
	if len(mac) <= 5 {
		return mac
	}
	return mac[len(mac)-5:]
}

func newUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	part1 := hex.EncodeToString(b[0:4])
	part2 := hex.EncodeToString(b[4:6])
	part3 := hex.EncodeToString(b[6:8])
	part4 := hex.EncodeToString(b[8:10])
	part5 := hex.EncodeToString(b[10:16])
	return fmt.Sprintf("%s-%s-%s-%s-%s", part1, part2, part3, part4, part5)
}

func cloneDeviceRecord(r *DeviceRecord) *DeviceRecord {
	if r == nil {
		return nil
	}
	out := *r
	out.Temperatures = append([]float64(nil), r.Temperatures...)
	if r.sensorHistory != nil {
		out.sensorHistory = make(map[int][]TrendPoint, len(r.sensorHistory))
		for sensor, points := range r.sensorHistory {
			out.sensorHistory[sensor] = append([]TrendPoint(nil), points...)
		}
	}
	return &out
}

func isRealTrendTemperature(value float64) bool {
	return value != 0 && value != 6553.5
}

func trimTrendPoints(points []TrendPoint, cutoff time.Time) []TrendPoint {
	idx := 0
	for idx < len(points) && points[idx].Timestamp.Before(cutoff) {
		idx++
	}
	if idx == 0 {
		return points
	}
	return append([]TrendPoint(nil), points[idx:]...)
}

// Load reads registry.json; initialises empty maps if file doesn't exist.
func (r *Registry) Load(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.path = path
	r.config = defaultAppConfig()
	r.devices = make(map[string]*DeviceRecord)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var file RegistryFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}

	r.config = normalizeAppConfig(file.Config)
	for mac, cfg := range file.Devices {
		normMAC := normalizeMAC(mac)
		cfg.MAC = normalizeMAC(cfg.MAC)
		if cfg.MAC == "" {
			cfg.MAC = normMAC
		}
		if cfg.Name == "" {
			cfg.Name = defaultDeviceName(cfg.MAC)
		}
		if cfg.PollIntervalSec <= 0 {
			cfg.PollIntervalSec = r.config.DefaultPollIntervalSec
		}
		if cfg.UID == "" {
			cfg.UID = newUID()
		}
		r.devices[normMAC] = &DeviceRecord{
			Config:       cfg,
			Status:       "Disconnected",
			BatteryLevel: -1,
			sensorHistory: make(map[int][]TrendPoint),
		}
	}

	return nil
}

// Save marshals DeviceConfig for all devices + AppConfig to registry.json atomically.
func (r *Registry) Save() error {
	r.mu.RLock()
	file := RegistryFile{
		Config:  normalizeAppConfig(r.config),
		Devices: make(map[string]DeviceConfig, len(r.devices)),
	}
	for mac, record := range r.devices {
		file.Devices[mac] = record.Config
	}
	path := r.path
	r.mu.RUnlock()

	if path == "" {
		return fmt.Errorf("registry path is not set")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// RegisterDevice creates a new record for a MAC if needed and returns a snapshot.
func (r *Registry) RegisterDevice(mac string) *DeviceRecord {
	mac = normalizeMAC(mac)
	if mac == "" {
		return nil
	}

	r.mu.Lock()
	if existing, ok := r.devices[mac]; ok {
		out := cloneDeviceRecord(existing)
		r.mu.Unlock()
		return out
	}

	cfg := DeviceConfig{
		MAC:             mac,
		UID:             newUID(),
		Name:            defaultDeviceName(mac),
		PollIntervalSec: normalizeAppConfig(r.config).DefaultPollIntervalSec,
		FirstSeen:       time.Now().UTC(),
	}
	record := &DeviceRecord{
		Config:       cfg,
		Status:       "Disconnected",
		BatteryLevel: -1,
		sensorHistory: make(map[int][]TrendPoint),
	}
	r.devices[mac] = record
	r.mu.Unlock()

	if err := r.Save(); err != nil {
		logger.Error("Failed to save registry after registering device", "mac", mac, "err", err)
	}
	return cloneDeviceRecord(record)
}

// Get returns a snapshot of a device record.
func (r *Registry) Get(mac string) *DeviceRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneDeviceRecord(r.devices[normalizeMAC(mac)])
}

// All returns snapshots of all device records.
func (r *Registry) All() []*DeviceRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()

	macs := make([]string, 0, len(r.devices))
	for mac := range r.devices {
		macs = append(macs, mac)
	}
	sort.Strings(macs)

	out := make([]*DeviceRecord, 0, len(macs))
	for _, mac := range macs {
		out = append(out, cloneDeviceRecord(r.devices[mac]))
	}
	return out
}

func (r *Registry) SetName(mac, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}

	mac = normalizeMAC(mac)
	r.mu.Lock()
	record, ok := r.devices[mac]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("unknown device %s", mac)
	}
	record.Config.Name = name
	r.mu.Unlock()

	return r.Save()
}

func (r *Registry) SetPollInterval(mac string, sec int) error {
	if sec < 1 {
		return fmt.Errorf("poll interval must be at least 1 second")
	}

	mac = normalizeMAC(mac)
	r.mu.Lock()
	record, ok := r.devices[mac]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("unknown device %s", mac)
	}
	record.Config.PollIntervalSec = sec
	r.mu.Unlock()

	return r.Save()
}

func (r *Registry) SetStatus(mac, status string) {
	mac = normalizeMAC(mac)
	r.mu.Lock()
	defer r.mu.Unlock()
	if record, ok := r.devices[mac]; ok {
		record.Status = status
	}
}

func (r *Registry) UpdateReadings(mac string, temps []float64, battery int) {
	mac = normalizeMAC(mac)
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.devices[mac]
	if !ok {
		return
	}
	if temps != nil {
		record.Temperatures = append([]float64(nil), temps...)
		now := time.Now().UTC()
		cutoff := now.Add(-trendRetention)
		if record.sensorHistory == nil {
			record.sensorHistory = make(map[int][]TrendPoint)
		}
		for sensor, value := range temps {
			if !isRealTrendTemperature(value) {
				continue
			}
			points := append(record.sensorHistory[sensor], TrendPoint{
				Timestamp: now,
				Value:     value,
			})
			record.sensorHistory[sensor] = trimTrendPoints(points, cutoff)
		}
		for sensor, points := range record.sensorHistory {
			record.sensorHistory[sensor] = trimTrendPoints(points, cutoff)
		}
	}
	if battery >= 0 {
		record.BatteryLevel = battery
	}
	record.LastSeen = time.Now().UTC()
}

func (r *Registry) SetConfig(cfg AppConfig) error {
	cfg = normalizeAppConfig(cfg)

	r.mu.Lock()
	r.config = cfg
	for _, record := range r.devices {
		if record.Config.PollIntervalSec <= 0 {
			record.Config.PollIntervalSec = cfg.DefaultPollIntervalSec
		}
	}
	r.mu.Unlock()

	return r.Save()
}

func (r *Registry) Config() AppConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return normalizeAppConfig(r.config)
}

func (r *Registry) DeviceName(mac string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	record, ok := r.devices[normalizeMAC(mac)]
	if !ok {
		return "", false
	}
	return record.Config.Name, true
}

func (r *Registry) TrendSeries() []TrendSeries {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cutoff := time.Now().UTC().Add(-trendRetention)
	macs := make([]string, 0, len(r.devices))
	for mac := range r.devices {
		macs = append(macs, mac)
	}
	sort.Strings(macs)

	var series []TrendSeries
	for _, mac := range macs {
		record := r.devices[mac]
		if record == nil || len(record.sensorHistory) == 0 {
			continue
		}
		sensors := make([]int, 0, len(record.sensorHistory))
		for sensor := range record.sensorHistory {
			sensors = append(sensors, sensor)
		}
		sort.Ints(sensors)
		for _, sensor := range sensors {
			points := trimTrendPoints(record.sensorHistory[sensor], cutoff)
			if len(points) == 0 {
				continue
			}
			series = append(series, TrendSeries{
				Name:   fmt.Sprintf("%s-%d", record.Config.Name, sensor+1),
				Points: append([]TrendPoint(nil), points...),
			})
		}
	}
	return series
}

func (r *Registry) SetMQTTConnected(connected bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mqttConnected = connected
}

func (r *Registry) RecordMQTTPublish(topic string, payload interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record := MQTTPublishRecord{
		Timestamp: time.Now().UTC(),
		Topic:     topic,
		Payload:   fmt.Sprint(payload),
	}
	r.recentPublishes = append([]MQTTPublishRecord{record}, r.recentPublishes...)
	if len(r.recentPublishes) > 5 {
		r.recentPublishes = r.recentPublishes[:5]
	}
}

func (r *Registry) MQTTConnected() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mqttConnected
}

func (r *Registry) RecentMQTTPublishes() []MQTTPublishRecord {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]MQTTPublishRecord(nil), r.recentPublishes...)
}

func (r *Registry) ShouldPublishTemperature(mac string) (string, bool) {
	mac = normalizeMAC(mac)
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.devices[mac]
	if !ok {
		return "", false
	}
	intervalSec := record.Config.PollIntervalSec
	if intervalSec <= 0 {
		intervalSec = normalizeAppConfig(r.config).DefaultPollIntervalSec
	}
	if time.Since(record.lastPublished) < time.Duration(intervalSec)*time.Second {
		return record.Config.Name, false
	}
	record.lastPublished = time.Now()
	return record.Config.Name, true
}
