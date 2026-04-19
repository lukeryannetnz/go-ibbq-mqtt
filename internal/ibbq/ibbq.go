/*
Copyright 2018 the original author or authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package ibbq

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-ble/ble"
)

// Ibbq is an instance of the thermometer
type Ibbq struct {
	ctx                         context.Context
	config                      Configuration
	disconnectedHandler         DisconnectedHandler
	temperatureReceivedHandler  TemperatureReceivedHandler
	batteryLevelReceivedHandler BatteryLevelReceivedHandler
	statusUpdatedHandler        StatusUpdatedHandler
	client                      ble.Client
	profile                     *ble.Profile
	disconnected                chan struct{}
	status                      Status
}

// TemperatureReceivedHandler is a callback for temperature readings.
// All temperature readings are returned in celsius.
type TemperatureReceivedHandler func([]float64)

// BatteryLevelReceivedHandler is a callback for battery readings.
// All battery readings are returned as percentages.
type BatteryLevelReceivedHandler func(int)

// DisconnectedHandler handles disconnection events
type DisconnectedHandler func()

// StatusUpdatedHandler is a callback for status updates.
type StatusUpdatedHandler func(Status)

var (
	bleInitOnce sync.Once
	bleInitErr  error
)

// InitBLE initializes the process-wide default BLE device once.
func InitBLE() error {
	bleInitOnce.Do(func() {
		var d ble.Device
		d, bleInitErr = NewDevice("default")
		if bleInitErr != nil {
			return
		}
		ble.SetDefaultDevice(d)
	})
	return bleInitErr
}

// NewIbbq creates a new Ibbq.
func NewIbbq(ctx context.Context, config Configuration, disconnectedHandler DisconnectedHandler, temperatureReceivedHandler TemperatureReceivedHandler, batteryLevelReceivedHandler BatteryLevelReceivedHandler, statusUpdatedHandler StatusUpdatedHandler) (ibbq Ibbq, err error) {
	return Ibbq{ctx, config, disconnectedHandler, temperatureReceivedHandler, batteryLevelReceivedHandler, statusUpdatedHandler, nil, nil, nil, Disconnected}, nil
}

func (ibbq *Ibbq) handleDisconnects() {
	logger.Debug("waiting for disconnect")
	<-ibbq.client.Disconnected()
	logger.Info("Disconnected", "addr", ibbq.client.Addr().String())
	ibbq.client = nil
	ibbq.profile = nil
	ibbq.updateStatus(Disconnected)
	go ibbq.disconnectedHandler()
}

func (ibbq *Ibbq) handleContextClosed() {
	logger.Debug("waiting for context to close")
	<-ibbq.ctx.Done()
	ibbq.Disconnect(false)
}

// Connect connects to an ibbq
func (ibbq *Ibbq) Connect() error {
	var client ble.Client
	var err error
	timeoutContext, cancel := context.WithTimeout(ibbq.ctx, ibbq.config.ConnectTimeout)
	defer cancel()
	c := make(chan interface{})
	logger.Info("Connecting to device")
	go func() {
		ibbq.updateStatus(Connecting)
		if client, err = ble.Connect(timeoutContext, ibbq.filter); err == nil {
			logger.Info("Connected to device", "addr", client.Addr())
			ibbq.client = client
			logger.Debug("Setting up disconnect handler")
			go ibbq.handleDisconnects()
			logger.Debug("Setting up context closed handler")
			go ibbq.handleContextClosed()
			err = ibbq.discoverProfile()
		}
		if err == nil {
			err = ibbq.login()
		}
		if err == nil {
			err = ibbq.subscribeToSettingResults()
		}
		if err == nil {
			err = ibbq.ConfigureTemperatureCelsius()
		}
		if err == nil {
			err = ibbq.subscribeToRealTimeData()
		}
		if err == nil {
			err = ibbq.subscribeToHistoryData()
		}
		if err == nil {
			err = ibbq.enableRealTimeData()
		}
		if err == nil {
			err = ibbq.enableBatteryData()
		}
		c <- err
		close(c)
	}()
	select {
	case <-timeoutContext.Done():
		logger.Error("timeout while connecting")
		err = timeoutContext.Err()
		ibbq.updateStatus(Disconnected)
	case err := <-c:
		if err != nil {
			logger.Error("Error received while connecting", "err", err)
			ibbq.updateStatus(Disconnected)
		} else {
			ibbq.updateStatus(Connected)
		}
	}
	return err
}

func (ibbq *Ibbq) discoverProfile() error {
	var profile *ble.Profile
	var err error
	if profile, err = ibbq.client.DiscoverProfile(true); err == nil {
		ibbq.profile = profile
	}
	return err
}

func (ibbq *Ibbq) login() error {
	var err error
	var uuid ble.UUID
	if uuid, err = ble.Parse(AccountAndVerify); err == nil {
		logger.Debug("logging in to device", "addr", ibbq.client.Addr(), "uuid", uuid)
		characteristic := ble.NewCharacteristic(uuid)
		if c := ibbq.profile.FindCharacteristic(characteristic); c != nil {
			err = ibbq.client.WriteCharacteristic(c, Credentials, false)
			logger.Debug("credentials written")
		}
	}
	return err
}

func (ibbq *Ibbq) updateStatus(status Status) {
	ibbq.status = status
	if ibbq.statusUpdatedHandler != nil {
		go ibbq.statusUpdatedHandler(status)
	}
}

func (ibbq *Ibbq) subscribeToRealTimeData() error {
	var err error
	var uuid ble.UUID
	logger.Info("Subscribing to real-time data")
	if uuid, err = ble.Parse(RealTimeData); err == nil {
		characteristic := ble.NewCharacteristic(uuid)
		if c := ibbq.profile.FindCharacteristic(characteristic); c != nil {
			err = ibbq.client.Subscribe(c, false, ibbq.realTimeDataReceived())
			if err == nil {
				logger.Info("Subscribed to real-time data")
			} else {
				logger.Error("Error subscribing to real-time data", "err", err)
			}
		} else {
			err = errors.New("can't find characteristic for real-time data")
		}
	}
	return err
}

func (ibbq *Ibbq) realTimeDataReceived() ble.NotificationHandler {
	return func(data []byte) {
		logger.Debug("received real-time data", hex.EncodeToString(data))
		var probeData []float64
		for i := 0; i+1 < len(data); i += 2 {
			raw := binary.LittleEndian.Uint16(data[i : i+2])
			if raw == 0xFFF6 {
				continue
			}
			probeData = append(probeData, float64(raw)/10)
		}
		if len(probeData) == 0 {
			return
		}
		go ibbq.temperatureReceivedHandler(probeData)
	}
}

func (ibbq *Ibbq) subscribeToHistoryData() error {
	var err error
	var uuid ble.UUID
	logger.Info("Subscribing to history data")
	if uuid, err = ble.Parse(HistoryData); err == nil {
		characteristic := ble.NewCharacteristic(uuid)
		if c := ibbq.profile.FindCharacteristic(characteristic); c != nil {
			err = ibbq.client.Subscribe(c, false, ibbq.historyDataReceived())
			if err == nil {
				logger.Info("Subscribed to history data")
			} else {
				logger.Error("Error subscribing to history data", "err", err)
			}
		} else {
			err = errors.New("Can't find characteristic for history data")
		}
	}
	return err
}

func (ibbq *Ibbq) historyDataReceived() ble.NotificationHandler {
	return func(data []byte) {
		logger.Debug("received history data", hex.EncodeToString(data))
	}
}

func (ibbq *Ibbq) subscribeToSettingResults() error {
	var err error
	var uuid ble.UUID
	logger.Info("Subscribing to setting results")
	if uuid, err = ble.Parse(SettingResult); err == nil {
		characteristic := ble.NewCharacteristic(uuid)
		if c := ibbq.profile.FindCharacteristic(characteristic); c != nil {
			err = ibbq.client.Subscribe(c, false, ibbq.settingResultReceived())
			if err == nil {
				logger.Info("Subscribed to setting results")
			} else {
				logger.Error("Error subscribing to setting results", "err", err)
			}
		} else {
			err = errors.New("Can't find characteristic for setting results")
		}
	}
	return err
}

func (ibbq *Ibbq) settingResultReceived() ble.NotificationHandler {
	return func(data []byte) {
		logger.Debug("Received setting result", "data", hex.EncodeToString(data))
		if len(data) == 0 {
			logger.Error("Received empty setting result payload")
			return
		}
		switch data[0] {
		case 0x24:
			logger.Debug("battery raw", "data", hex.EncodeToString(data))
			batteryPct, err := parseBatteryPercentage(data)
			if err != nil {
				logger.Error("Unable to parse battery level", "err", err)
				return
			}
			go ibbq.batteryLevelReceivedHandler(batteryPct)
		}
	}
}

func (ibbq *Ibbq) enableRealTimeData() error {
	logger.Info("Enabling real-time data sending")
	err := ibbq.writeSetting(realTimeDataEnable)
	if err == nil {
		logger.Info("Enabled real-time data sending")
	}
	return err
}

func (ibbq *Ibbq) enableBatteryData() error {
	if ibbq.config.BatteryPollingInterval > 0 {
		logger.Info("Enabling battery data sending")
		var err error
		if err = ibbq.writeSetting(batteryLevel); err == nil {
			ticker := time.NewTicker(ibbq.config.BatteryPollingInterval)
			go func() {
				for {
					select {
					case <-ticker.C:
						logger.Debug("Requesting battery data")
						err := ibbq.writeSetting(batteryLevel)
						if err != nil {
							logger.Error("Unable to request battery level", "err", err)
							ticker.Stop()
							return
						}
					case <-ibbq.client.Disconnected():
						ticker.Stop()
						return
					}
				}
			}()
		}
		return err
	}
	logger.Debug("Battery level polling was not enabled in configuration")
	return nil
}

// ConfigureTemperatureCelsius changes the device to display temperatures in Celsius on the screen.
// It does not change the units sent back over the wire, however, which are always in Celsius.
func (ibbq *Ibbq) ConfigureTemperatureCelsius() error {
	logger.Info("Configuring temperature for Celsius")
	err := ibbq.writeSetting(unitsCelsius)
	if err == nil {
		logger.Info("Configured temperature for Celsius")
	}
	return err
}

// ConfigureTemperatureFahrenheit changes the device to display temperatures in Fahrenheit on the screen.
// It does not change the units sent back over the wire, however, which are always in Celsius.
func (ibbq *Ibbq) ConfigureTemperatureFahrenheit() error {
	logger.Info("Configuring temperature for Fahrenheit")
	err := ibbq.writeSetting(unitsFahrenheit)
	if err == nil {
		logger.Info("Configured temperature for Fahrenheit")
	}
	return err
}

func (ibbq *Ibbq) writeSetting(settingValue []byte) error {
	var err error
	var uuid ble.UUID
	if uuid, err = ble.Parse(SettingData); err == nil {
		characteristic := ble.NewCharacteristic(uuid)
		if c := ibbq.profile.FindCharacteristic(characteristic); c != nil {
			err = ibbq.client.WriteCharacteristic(c, settingValue, false)
		} else {
			err = errors.New("Can't find characteristic for settings data")
		}
	}
	return err
}

// Disconnect disconnects from an ibbq
func (ibbq *Ibbq) Disconnect(force bool) error {
	var err error
	if ibbq.client == nil {
		err = errors.New("Not connected")
		if force {
			ibbq.client = nil
			ibbq.profile = nil
			err = nil
			ibbq.updateStatus(Disconnected)
			go ibbq.disconnectedHandler()
		}
	} else {
		logger.Info("Disconnecting")
		ibbq.updateStatus(Disconnecting)
		err = ibbq.client.CancelConnection()
		if force {
			ibbq.client = nil
			ibbq.profile = nil
			err = nil
			ibbq.updateStatus(Disconnected)
			go ibbq.disconnectedHandler()
		}
	}
	return err
}

func (ibbq *Ibbq) filter(a ble.Advertisement) bool {
	if !IsSupportedDeviceName(a.LocalName()) || !a.Connectable() {
		return false
	}
	if ibbq.config.TargetMAC == "" {
		return true
	}
	return strings.EqualFold(a.Addr().String(), ibbq.config.TargetMAC)
}

func advHandler() ble.AdvHandler {
	return func(a ble.Advertisement) {
		logger.Debug("Found advertisement",
			"address", a.Addr(),
			"connectable", a.Connectable(),
			"rssi", a.RSSI(),
			"name", a.LocalName(),
			"svcs", a.Services(),
			"manufacturerData", a.ManufacturerData())
	}
}

func parseBatteryPercentage(data []byte) (int, error) {
	if len(data) < 5 {
		return 0, fmt.Errorf("battery payload too short: %d", len(data))
	}

	littleCurrent := int(binary.LittleEndian.Uint16(data[1:3]))
	littleMax := int(binary.LittleEndian.Uint16(data[3:5]))
	if pct, ok := batteryPercentageFromVoltages(littleCurrent, littleMax); ok {
		return pct, nil
	}

	bigCurrent := int(binary.BigEndian.Uint16(data[1:3]))
	bigMax := int(binary.BigEndian.Uint16(data[3:5]))
	if pct, ok := batteryPercentageFromVoltages(bigCurrent, bigMax); ok {
		logger.Warn("Battery payload matched big-endian fallback",
			"data", hex.EncodeToString(data),
			"littleCurrent", littleCurrent,
			"littleMax", littleMax,
			"bigCurrent", bigCurrent,
			"bigMax", bigMax)
		return pct, nil
	}

	return 0, fmt.Errorf("invalid battery payload: data=%s little=%d/%d big=%d/%d",
		hex.EncodeToString(data), littleCurrent, littleMax, bigCurrent, bigMax)
}

func batteryPercentageFromVoltages(currentVoltage int, maxVoltage int) (int, bool) {
	if maxVoltage == 0 {
		return 0, false
	}

	batteryPct := 100 * currentVoltage / maxVoltage
	if batteryPct < 0 || batteryPct > 100 {
		return 0, false
	}

	return batteryPct, true
}
