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
	"os"

	"github.com/go-ble/ble"
	"github.com/joho/godotenv"
	ibbq "github.com/lukeryannetnz/go-ibbq-mqtt/internal/ibbq"
	log "github.com/mgutz/logxi/v1"
)

const registryPath = "/var/lib/go-ibbq-mqtt/registry.json"

var logger = log.New("main")
var mc = NewMqttClient()

func configureEnv() {
	err := godotenv.Load()
	if err != nil {
		if os.IsNotExist(err) && os.Getenv("MQTT_SERVER") != "" {
			return
		}
		logger.Warn("No .env file found, using environment variables", "err", err)
	}
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

	if err := ibbq.InitBLE(); err != nil {
		logger.Fatal("BLE init failed", "err", err)
	}

	registry := &Registry{}
	if err := registry.Load(registryPath); err != nil {
		logger.Fatal("Failed to load registry", "err", err)
	}

	ctx1, cancel := context.WithCancel(context.Background())
	defer cancel()
	registerInterruptHandler(cancel, ctx1)
	ctx := ble.WithSigHandler(ctx1, cancel)

	mc.Init()

	port := os.Getenv("WEB_PORT")
	if port == "" {
		port = "8080"
	}

	dm := NewDeviceManager(registry, mc)
	go startWebServer(port, registry, dm)
	go dm.Run(ctx)

	<-ctx.Done()
	logger.Info("Exiting")
}
