# go-iBBQ MQTT Publisher Example
Inspired by sworisbreathing/go-ibbq, this is a simple app that connects to an iBBQ over BLE using sworisbreathing/go-ibbq. It publishes the data it receives to an MQTT topic using github.com/eclipse/paho.mqtt.golang 

## Building

### Linux

```bash
$ GOOS=linux go build
```

### OS X

```bash
$ GOOS=darwin go build
```

## Usage

### Configuration via env
See .env for the configuration values you can set via the environment. The defaults in .env will be used if you don't override these.

### example terminal output
```bash
$ LOGXI=*=INF ./go-ibbq-mqtt
19:35:28.768196 INF main Connecting to mqtt broker broker: tcp://mqtt.local:1883
19:35:28.768491 INF main Connected to mqtt
19:35:28.768657 INF main Connecting to device
19:35:28.768876 INF ibbq Connecting to device
19:35:28.769377 INF main Status updated status: Connecting
19:35:28.770498 INF main Publishing to mqtt topic: ibbq/status
19:35:32.326019 INF ibbq Connected to device addr: 24:7d:4d:6a:8d:6e
19:35:32.649607 INF ibbq Subscribed to setting results
19:35:32.649815 INF ibbq Configuring temperature for Celsius
19:35:32.664590 INF ibbq Configured temperature for Celsius
19:35:32.664805 INF ibbq Subscribing to real-time data
19:35:32.679402 INF ibbq Subscribed to real-time data
19:35:32.679570 INF ibbq Subscribing to history data
19:35:32.694545 INF ibbq Subscribed to history data
19:35:32.694766 INF ibbq Enabling real-time data sending
19:35:32.709506 INF ibbq Enabled real-time data sending
19:35:32.709879 INF ibbq Enabling battery data sending
19:35:32.724509 INF main Connected to device
19:35:32.724585 INF main Status updated status: Connected
19:35:32.725073 INF main Publishing to mqtt topic: ibbq/status
19:35:32.739876 INF main Received battery data batteryPct: 69
19:35:32.740294 INF main Publishing to mqtt topic: ibbq/batterylevel
19:35:34.060102 INF main Received temperature data temperatures: [25 24]
19:35:34.060792 INF main Publishing to mqtt topic: ibbq/temperatures
19:35:36.284006 INF main Received temperature data temperatures: [25 24]
19:35:36.284516 INF main Publishing to mqtt topic: ibbq/temperatures
$
```
