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
	"strconv"
	"time"

	"github.com/go-ble/ble"
	log "github.com/mgutz/logxi/v1"
	"github.com/sworisbreathing/go-ibbq/v2"
)

var logger = log.New("main")
var mc = NewMqttClient()

func temperatureReceived(temperatures []float64) {
	logger.Info("Received temperature data", "temperatures", temperatures)
	mc.Pub(fmt.Sprintln("temps : {0}", temperatures))
}
func batteryLevelReceived(batteryLevel int) {
	logger.Info("Received battery data", "batteryPct", strconv.Itoa(batteryLevel))
	mc.Pub(fmt.Sprintln("batteryLevel : {0}", batteryLevel))
}
func statusUpdated(status ibbq.Status) {
	logger.Info("Status updated", "status", status)
	mc.Pub(fmt.Sprintln("status : {0}", status))
}

func disconnectedHandler(cancel func(), done chan struct{}) func() {
	return func() {
		logger.Info("Disconnected")
		cancel()
		close(done)
	}
}

func main() {
	var err error
	logger.Debug("initializing context")
	ctx1, cancel := context.WithCancel(context.Background())
	defer cancel()
	registerInterruptHandler(cancel)
	ctx := ble.WithSigHandler(ctx1, cancel)
	logger.Debug("context initialized")
	var bbq ibbq.Ibbq
	logger.Debug("instantiating ibbq struct")
	done := make(chan struct{})
	var config ibbq.Configuration
	if config, err = ibbq.NewConfiguration(60*time.Second, 5*time.Minute); err != nil {
		logger.Fatal("Error creating configuration", "err", err)
	}
	if bbq, err = ibbq.NewIbbq(ctx, config, disconnectedHandler(cancel, done), temperatureReceived, batteryLevelReceived, statusUpdated); err != nil {
		logger.Fatal("Error creating iBBQ", "err", err)
	}
	logger.Debug("instantiated ibbq struct")

	mc.Init()
	logger.Info("Connected to mqtt")

	logger.Info("Connecting to device")
	if err = bbq.Connect(); err != nil {
		logger.Fatal("Error connecting to device", "err", err)
	}
	logger.Info("Connected to device")

	<-ctx.Done()
	<-done
	logger.Info("Exiting")
}
