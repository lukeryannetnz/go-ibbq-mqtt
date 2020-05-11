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
	"strconv"
	"time"

	"github.com/go-ble/ble"
	"github.com/joho/godotenv"
	log "github.com/mgutz/logxi/v1"
	"github.com/sworisbreathing/go-ibbq/v2"
)

var logger = log.New("main")
var mc = NewMqttClient()

func temperatureReceived(temperatures []float64) {
	logger.Info("Received temperature data", "temperatures", temperatures)

	t := &temperature{temperatures}
	mc.Pub("temperatures", t.toJson())
}

func batteryLevelReceived(level int) {
	logger.Info("Received battery data", "batteryPct", strconv.Itoa(level))

	b := &batteryLevel{level}
	mc.Pub("batterylevel", b.toJson())
}

func statusUpdated(ibbqStatus ibbq.Status) {
	logger.Info("Status updated", "status", ibbqStatus)

	s := &status{string(ibbqStatus)}
	mc.Pub("status", s.toJson())
}

func disconnectedHandler(cancel func(), done chan struct{}) func() {
	return func() {
		logger.Info("Disconnected")
		cancel()
		close(done)
	}
}

func configureEnv() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file", "err", err)
	}
}

func initializeiBbq(ctx context.Context, cancel context.CancelFunc, done chan struct{}) {
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
	logger.Info(`
	_____ ____        _  ____  ____  ____        _      ____  _____  _____ 
	/  __//  _ \      / \/  _ \/  _ \/  _ \      / \__/|/  _ \/__ __\/__ __\
	| |  _| / \|_____ | || | //| | //| / \|_____ | |\/||| / \|  / \    / \  
	| |_//| \_/|\____\| || |_\\| |_\\| \_\|\____\| |  ||| \_\|  | |    | |  
	\____\\____/      \_/\____/\____/\____\      \_/  \|\____\  \_/    \_/  
																	
`)
	configureEnv()

	logger.Debug("initializing context")
	ctx1, cancel := context.WithCancel(context.Background())
	defer cancel()
	registerInterruptHandler(cancel, ctx1)
	ctx := ble.WithSigHandler(ctx1, cancel)
	logger.Debug("context initialized")

	mc.Init()

	done := make(chan struct{})
	initializeiBbq(ctx, cancel, done)

	<-ctx.Done()
	<-done
	logger.Info("Exiting")
}
