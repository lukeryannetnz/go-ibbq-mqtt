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
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-ble/ble"
	"github.com/joho/godotenv"
	ibbq "github.com/lukeryannetnz/go-ibbq-mqtt/internal/ibbq"
	log "github.com/mgutz/logxi/v1"
)

var logger = log.New("main")
var mc = NewMqttClient()

type deviceConfig struct {
	mac  string
	name string
}

func temperatureReceived(deviceName string, temperatures []float64) {
	logger.Info("Received temperature data", "device", deviceName, "temperatures", temperatures)

	t := &temperature{temperatures}
	mc.Pub(deviceName, "temperatures", t.toJson())

	s := getOrCreateDeviceState(deviceName)
	s.mu.Lock()
	s.Temperatures = append([]float64(nil), temperatures...)
	s.LastSeen = time.Now()
	s.mu.Unlock()
}

func batteryLevelReceived(deviceName string, level int) {
	logger.Info("Received battery data", "device", deviceName, "batteryPct", strconv.Itoa(level))

	b := &batteryLevel{level}
	mc.Pub(deviceName, "batterylevel", b.toJson())

	s := getOrCreateDeviceState(deviceName)
	s.mu.Lock()
	s.BatteryLevel = level
	s.LastSeen = time.Now()
	s.mu.Unlock()
}

func statusUpdated(deviceName string, ibbqStatus ibbq.Status) {
	logger.Info("Status updated", "device", deviceName, "status", ibbqStatus)
	publishStatus(deviceName, string(ibbqStatus))

	s := getOrCreateDeviceState(deviceName)
	s.mu.Lock()
	s.Status = string(ibbqStatus)
	s.LastSeen = time.Now()
	s.mu.Unlock()
}

func configureEnv() {
	err := godotenv.Load()
	if err != nil {
		if os.IsNotExist(err) && os.Getenv("MQTT_SERVER") != "" {
			return
		}
		logger.Warn("No .env file found, using environment variables", "err", err)
	}
}

func splitTrimmed(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func readDeviceConfigs() []deviceConfig {
	namesStr := os.Getenv("DEVICE_NAMES")
	macsStr := os.Getenv("DEVICE_MACS")
	if namesStr == "" && macsStr == "" {
		name := os.Getenv("DEVICE_NAME")
		if name == "" {
			name = "default"
		}
		return []deviceConfig{{
			mac:  os.Getenv("DEVICE_MAC"),
			name: name,
		}}
	}

	names := splitTrimmed(namesStr)
	macs := splitTrimmed(macsStr)
	for i := len(names); i < len(macs); i++ {
		names = append(names, fmt.Sprintf("device%d", i+1))
	}

	configs := make([]deviceConfig, len(macs))
	for i := range macs {
		name := fmt.Sprintf("device%d", i+1)
		if i < len(names) && names[i] != "" {
			name = names[i]
		}
		configs[i] = deviceConfig{
			mac:  macs[i],
			name: name,
		}
	}
	return configs
}

func connectWithRetry(ctx context.Context, dev deviceConfig) {
	attempts := 0
	retryDelays := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		done := make(chan struct{})
		err := tryConnect(ctx, dev, done)
		if err != nil {
			attempts++
			logger.Error("Connection failed", "device", dev.name, "err", err, "attempt", attempts)
			publishStatus(dev.name, "Disconnected")

			delay := 60 * time.Second
			if attempts-1 < len(retryDelays) {
				delay = retryDelays[attempts-1]
			}

			logger.Info("Retrying", "device", dev.name, "delay", delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}

		attempts = 0
		select {
		case <-ctx.Done():
			return
		case <-done:
			logger.Info("Disconnected, will reconnect", "device", dev.name)
			publishStatus(dev.name, "Disconnected")
		}
	}
}

func tryConnect(ctx context.Context, dev deviceConfig, done chan struct{}) error {
	config, err := ibbq.NewConfiguration(60*time.Second, 5*time.Minute)
	if err != nil {
		return err
	}

	config.TargetMAC = dev.mac

	bbq, err := ibbq.NewIbbq(
		ctx,
		config,
		func() {
			close(done)
		},
		func(temperatures []float64) {
			temperatureReceived(dev.name, temperatures)
		},
		func(level int) {
			batteryLevelReceived(dev.name, level)
		},
		func(ibbqStatus ibbq.Status) {
			statusUpdated(dev.name, ibbqStatus)
		},
	)
	if err != nil {
		return err
	}

	logger.Info("Connecting to device", "device", dev.name, "targetMAC", dev.mac)
	if err := bbq.Connect(); err != nil {
		return err
	}

	logger.Info("Connected to device", "device", dev.name)
	return nil
}

func main() {
	logger.Info(`
	_____ ____        _  ____  ____  ____        _      ____  _____  _____ 
	/  __//  _ \      / \/  _ \/  _ \/  _ \      / \__/|/  _ \/__ __\/__ __\
	| |  _| / \|_____ | || | //| | //| / \|_____ | |\/||| / \|  / \    / \  
	| |_//| \_/|\____\| || |_\\| |_\\| \_\|\____\| |  ||| \_\|  | |    | |  
	\____\\____/      \_/\____/\____/\____\      \_/  \|\____\  \_/    \_/  
																	
`)
	configureEnv()
	devs := readDeviceConfigs()

	logger.Debug("initializing context")
	ctx1, cancel := context.WithCancel(context.Background())
	defer cancel()
	registerInterruptHandler(cancel, ctx1)
	ctx := ble.WithSigHandler(ctx1, cancel)
	logger.Debug("context initialized")

	mc.Init()
	if err := ibbq.InitBLE(); err != nil {
		logger.Fatal("BLE init failed", "err", err)
	}
	port := os.Getenv("WEB_PORT")
	if port == "" {
		port = "8080"
	}
	go startWebServer(port)

	var wg sync.WaitGroup
	for _, dev := range devs {
		wg.Add(1)
		go func(d deviceConfig) {
			defer wg.Done()
			connectWithRetry(ctx, d)
		}(dev)
	}
	wg.Wait()
	logger.Info("Exiting")
}
