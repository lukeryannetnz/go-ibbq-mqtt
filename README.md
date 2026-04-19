# Inkbird wireless thermometer MQTT publisher 
![Go](https://github.com/lukeryannetnz/go-ibbq-mqtt/workflows/Go/badge.svg)

This project connects to an Inkbird wireless thermometer over Bluetooth and publishes readings to MQTT using a vendored `internal/ibbq` package plus [`paho.mqtt.golang`](https://github.com/eclipse/paho.mqtt.golang).

## Building

### Linux

```bash
$ GOOS=linux go build
```

> **Raspberry Pi / Debian / Ubuntu note:** If you install Go via `apt`, the `GOROOT` env var may not be set correctly (e.g. pointing to `/usr/local/go` which doesn't exist). Fix it by setting `GOROOT` explicitly:
> ```bash
> export GOROOT=/usr/lib/go-1.22
> export PATH=$GOROOT/bin:$PATH
> ```
> Add these lines to `~/.profile` or `~/.bashrc` to make them permanent.

### OS X

```bash
$ GOOS=darwin go build
```

## Usage

### New machine setup

For a fresh Raspberry Pi or Debian machine, install the OS packages, clone the repo, build the binary, and then install the service:

```bash
sudo apt update
sudo apt install -y git golang bluez

git clone http://gitea.bracken.life:3000/jaidan/ibbq-multi.git
cd ibbq-multi

GOOS=linux go build

sudo usermod -aG bluetooth "$USER"
newgrp bluetooth

chmod +x install.sh
./install.sh
sudo nano /etc/default/go-ibbq-mqtt
sudo systemctl restart go-ibbq-mqtt
sudo systemctl status go-ibbq-mqtt
```

The values in `/etc/default/go-ibbq-mqtt` are what the service will use on boot. Editing `.env` in the repo only affects manual runs from the checkout directory.

If you prefer to install manually instead of using the script:

```bash
cp .env.example .env
nano .env

sudo useradd -r -s /usr/sbin/nologin ibbq || true
sudo install -m 0755 go-ibbq-mqtt /usr/local/bin/go-ibbq-mqtt
sudo cp .env.example /etc/default/go-ibbq-mqtt
sudo nano /etc/default/go-ibbq-mqtt
sudo cp go-ibbq-mqtt.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now go-ibbq-mqtt
sudo systemctl status go-ibbq-mqtt
```

### Configuration via env
Copy `.env.example` to `.env` and edit the values before running:

```bash
cp .env.example .env
nano .env
```

Key settings:
- `MQTT_SERVER` — address of your MQTT broker (e.g. `tcp://localhost:1883`)
- `DEVICE_MAC` — Bluetooth MAC of your Inkbird (leave blank to auto-discover)
- `DEVICE_NAME` — label used in MQTT topics

### Running

```bash
LOGXI=*=INF ./go-ibbq-mqtt
```

### Raspberry Pi Bluetooth permissions

If the binary starts only with `sudo` and fails with an error like `hci0: can't down device: operation not permitted`, add your user to the `bluetooth` group and refresh the shell session:

```bash
sudo usermod -aG bluetooth "$USER"
newgrp bluetooth
```

Then run the binary again without `sudo`:

```bash
LOGXI=*=INF ./go-ibbq-mqtt
```

### Run on boot with systemd

This repo includes both a single-device unit (`go-ibbq-mqtt.service`) and a template unit for multiple thermometers (`go-ibbq-mqtt@.service`).

Install the binary, env file, and unit on the Pi:

```bash
./install.sh
```

Useful service commands:

```bash
sudo systemctl status go-ibbq-mqtt
sudo journalctl -u go-ibbq-mqtt -f
sudo systemctl restart go-ibbq-mqtt
```

The important bit is `systemctl enable`: that creates the boot-time symlink, so the service starts automatically on boot. `--now` also starts it immediately without waiting for a reboot.

For multiple devices, use the template unit and one env file per device:

```bash
sudo install -m 0755 go-ibbq-mqtt /usr/local/bin/go-ibbq-mqtt
sudo cp .env.probe1.example /etc/default/go-ibbq-mqtt.probe1
sudo cp .env.probe2.example /etc/default/go-ibbq-mqtt.probe2
sudo cp go-ibbq-mqtt@.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now go-ibbq-mqtt@probe1
sudo systemctl enable --now go-ibbq-mqtt@probe2
```

### example terminal output
```bash
$ LOGXI=*=INF ./go-ibbq-mqtt
19:35:28.768185 INF main
   	_____ ____        _  ____  ____  ____        _      ____  _____  _____
	/  __//  _ \      / \/  _ \/  _ \/  _ \      / \__/|/  _ \/__ __\/__ __\
	| |  _| / \|_____ | || | //| | //| / \|_____ | |\/||| / \|  / \    / \
	| |_//| \_/|\____\| || |_\\| |_\\| \_\|\____\| |  ||| \_\|  | |    | |
	\____\\____/      \_/\____/\____/\____\      \_/  \|\____\  \_/    \_/

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
