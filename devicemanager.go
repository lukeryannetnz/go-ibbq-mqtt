package main

import (
	"context"
	"sync"
	"time"

	ibbq "github.com/lukeryannetnz/go-ibbq-mqtt/internal/ibbq"
)

type DeviceManager struct {
	registry *Registry
	mc       MqttClient

	mu       sync.Mutex
	opMu     sync.Mutex
	cancels  map[string]context.CancelFunc
	rescanCh chan struct{}
}

func NewDeviceManager(registry *Registry, mc MqttClient) *DeviceManager {
	return &DeviceManager{
		registry: registry,
		mc:       mc,
		cancels:  make(map[string]context.CancelFunc),
		rescanCh: make(chan struct{}, 1),
	}
}

func (dm *DeviceManager) Run(ctx context.Context) {
	if err := dm.performScanAndConnect(ctx); err != nil {
		logger.Error("Initial BLE scan failed", "err", err)
	}

	go dm.runDeadChecker(ctx)
	go dm.runScanLoop(ctx)

	<-ctx.Done()
}

func (dm *DeviceManager) TriggerScan() {
	select {
	case dm.rescanCh <- struct{}{}:
	default:
	}
}

func (dm *DeviceManager) performScanAndConnect(ctx context.Context) error {
	logger.Info("Running BLE scan")
	macs, err := dm.scanForDevices(ctx, defaultDiscoveryScanDuration)
	if err != nil {
		return err
	}
	for _, mac := range macs {
		dm.registry.RegisterDevice(mac)
	}
	dm.connectSequential(ctx, macs)
	return nil
}

func (dm *DeviceManager) runScanLoop(ctx context.Context) {
	for {
		interval := time.Duration(dm.registry.Config().ScanIntervalSec) * time.Second
		timer := time.NewTimer(interval)

		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-dm.rescanCh:
			timer.Stop()
		case <-timer.C:
			logger.Info("Running periodic BLE scan")
		}

		if err := dm.performScanAndConnect(ctx); err != nil && ctx.Err() == nil {
			logger.Error("Periodic scan failed", "err", err)
		}
	}
}

func (dm *DeviceManager) scanForDevices(ctx context.Context, duration time.Duration) ([]string, error) {
	dm.opMu.Lock()
	defer dm.opMu.Unlock()
	return ScanForDevices(ctx, duration)
}

func (dm *DeviceManager) connectSequential(ctx context.Context, macs []string) {
	for _, mac := range macs {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if dm.isManaged(mac) {
			continue
		}

		done := make(chan struct{})
		if err := dm.tryConnect(ctx, mac, done); err != nil {
			logger.Error("Initial connect failed", "mac", mac, "err", err)
			dm.registry.SetStatus(mac, "Disconnected")
			close(done)
		}

		devCtx, cancel := context.WithCancel(ctx)
		dm.mu.Lock()
		dm.cancels[mac] = cancel
		dm.mu.Unlock()

		go dm.manageDevice(devCtx, mac, done)
	}
}

func (dm *DeviceManager) manageDevice(ctx context.Context, mac string, initialDone chan struct{}) {
	defer dm.removeCancelEntry(mac)

	done := initialDone
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			logger.Info("Device disconnected, will reconnect", "mac", mac)
			dm.registry.SetStatus(mac, "Disconnected")
		}

		attempts := 0
		delays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			done = make(chan struct{})
			if err := dm.tryConnect(ctx, mac, done); err == nil {
				break
			} else {
				attempts++
				delay := 60 * time.Second
				if attempts-1 < len(delays) {
					delay = delays[attempts-1]
				}
				logger.Error("Reconnect failed", "mac", mac, "attempt", attempts, "retryIn", delay, "err", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
		}
	}
}

func (dm *DeviceManager) tryConnect(ctx context.Context, mac string, done chan struct{}) error {
	config, err := ibbq.NewConfiguration(60*time.Second, 5*time.Minute)
	if err != nil {
		return err
	}
	config.TargetMAC = normalizeMAC(mac)

	bbq, err := ibbq.NewIbbq(
		ctx,
		config,
		func() { close(done) },
		func(temps []float64) { dm.onTemperature(mac, temps) },
		func(level int) { dm.onBattery(mac, level) },
		func(s ibbq.Status) { dm.onStatus(mac, s) },
	)
	if err != nil {
		return err
	}

	dm.registry.SetStatus(mac, "Connecting")

	dm.opMu.Lock()
	err = bbq.Connect()
	dm.opMu.Unlock()
	return err
}

func (dm *DeviceManager) onTemperature(mac string, temps []float64) {
	dm.registry.UpdateReadings(mac, temps, -1)
	name, shouldPublish := dm.registry.ShouldPublishTemperature(mac)
	if !shouldPublish {
		return
	}

	t := &temperature{temps}
	dm.mc.Pub(name, "temperatures", t.toJson())
}

func (dm *DeviceManager) onBattery(mac string, level int) {
	dm.registry.UpdateReadings(mac, nil, level)
	name, ok := dm.registry.DeviceName(mac)
	if !ok {
		return
	}
	b := &batteryLevel{level}
	dm.mc.Pub(name, "batterylevel", b.toJson())
}

func (dm *DeviceManager) onStatus(mac string, s ibbq.Status) {
	dm.registry.SetStatus(mac, string(s))
	name, ok := dm.registry.DeviceName(mac)
	if !ok {
		return
	}
	publishStatus(name, string(s))
}

func (dm *DeviceManager) runDeadChecker(ctx context.Context) {
	ticker := time.NewTicker(defaultDeadCheckerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, record := range dm.registry.All() {
				if record.Status != "Connected" || record.LastSeen.IsZero() {
					continue
				}
				if time.Since(record.LastSeen) <= defaultDeadAfter {
					continue
				}

				logger.Warn("Device has not sent data in 10 minutes, marking dead", "mac", record.Config.MAC)
				dm.registry.SetStatus(record.Config.MAC, "Dead")
				dm.mu.Lock()
				if cancel, ok := dm.cancels[record.Config.MAC]; ok {
					cancel()
					delete(dm.cancels, record.Config.MAC)
				}
				dm.mu.Unlock()
				publishStatus(record.Config.Name, "Dead")
			}
		}
	}
}

func (dm *DeviceManager) isManaged(mac string) bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	_, ok := dm.cancels[normalizeMAC(mac)]
	return ok
}

func (dm *DeviceManager) removeCancelEntry(mac string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	delete(dm.cancels, normalizeMAC(mac))
}
