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
	"github.com/joho/godotenv"
	log "github.com/mgutz/logxi/v1"
	"github.com/sworisbreathing/go-ibbq/v2"
)

var logger = log.New("main")
var mc = NewMqttClient()

func f64tostring(input []float64) string {
	return fmt.Sprintf("%f", input)
}

func inttostring(input int) string {
	return fmt.Sprintf("%d", input)
}

func temperatureReceived(temperatures []float64) {
	logger.Info("Received temperature data", "temperatures", temperatures)
	mc.Pub("temperatures", f64tostring(temperatures))
}

func batteryLevelReceived(batteryLevel int) {
	logger.Info("Received battery data", "batteryPct", strconv.Itoa(batteryLevel))
	mc.Pub("batterylevel", inttostring(batteryLevel))
}

func statusUpdated(status ibbq.Status) {
	logger.Info("Status updated", "status", status)
	mc.Pub("status", string(status))
}

func disconnectedHandler(cancel func(), done chan struct{}) func() {
	return func() {
		logger.Info("Disconnected")
		cancel()
		close(done)
	}
}

func configureenv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file", "err", err)
	}
}

func initializeibbq(ctx context.Context, cancel context.CancelFunc, done chan struct{}) {
	logger.Debug("instantiating ibbq structs")
	var err error
	var bbq ibbq.Ibbq
	var config ibbq.Configuration
	logger.Debug("instantiated ibbq structs")

	if config, err = ibbq.NewConfiguration(60*time.Second, 5*time.Minute); err != nil {
		logger.Fatal("Error creating configuration", "err", err)
	}

	logger.Info("Connecting to device")
	if bbq, err = ibbq.NewIbbq(ctx, config, disconnectedHandler(cancel, done), temperatureReceived, batteryLevelReceived, statusUpdated); err != nil {
		logger.Fatal("Error creating iBBQ", "err", err)
	}

	if err = bbq.Connect(); err != nil {
		logger.Fatal("Error connecting to device", "err", err)
	}
	logger.Info("Connected to device")
}

func main() {
	logger.Debug(`
	_____ ____        _  ____  ____  ____        _      ____  _____  _____ 
	/  __//  _ \      / \/  _ \/  _ \/  _ \      / \__/|/  _ \/__ __\/__ __\
	| |  _| / \|_____ | || | //| | //| / \|_____ | |\/||| / \|  / \    / \  
	| |_//| \_/|\____\| || |_\\| |_\\| \_\|\____\| |  ||| \_\|  | |    | |  
	\____\\____/      \_/\____/\____/\____\      \_/  \|\____\  \_/    \_/  
																	
`)
	configureenv()

	logger.Debug("initializing context")
	ctx1, cancel := context.WithCancel(context.Background())
	defer cancel()
	registerInterruptHandler(cancel, ctx1)
	ctx := ble.WithSigHandler(ctx1, cancel)
	logger.Debug("context initialized")

	mc.Init()

	done := make(chan struct{})
	initializeibbq(ctx, cancel, done)

	<-ctx.Done()
	<-done
	logger.Info("Exiting")
}
