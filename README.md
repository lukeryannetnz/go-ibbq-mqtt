# Inkbird wireless thermometer MQTT publisher 
![Go](https://github.com/lukeryannetnz/go-ibbq-mqtt/workflows/Go/badge.svg)

This project connects to an Inkbird wireless thermometer over Bluetooth and publishes readings to MQTT using a vendored `internal/ibbq` package plus [`paho.mqtt.golang`](https://github.com/eclipse/paho.mqtt.golang).

## Building

### Linux

```bash
$ go build
```

For cross-compiling to a Linux `armv6l` target such as a Raspberry Pi Zero:

```bash
GOOS=linux GOARCH=arm GOARM=6 go build
```

### OS X

```bash
$ go build
```

## Usage

### New machine setup

For a fresh Raspberry Pi or Debian machine, install the OS packages, clone the repo, and then install the service:

```bash
sudo apt update
sudo apt install -y git bluez curl

git clone http://gitea.bracken.life:3000/jaidan/ibbq-multi.git
cd ibbq-multi

sudo usermod -aG bluetooth "$USER"
newgrp bluetooth

chmod +x install.sh
./install.sh --install-go
sudo nano /etc/default/go-ibbq-mqtt
sudo systemctl restart go-ibbq-mqtt
sudo systemctl status go-ibbq-mqtt
```

If the machine already has Go `1.21+`, you can skip the Go tarball install and run:

```bash
./install.sh
```

On `armv6l`, the installer uses Go `1.21.13` because newer official releases do not provide Linux `armv6l` builds.

The values in `/etc/default/go-ibbq-mqtt` are what the service will use on boot. Editing `.env` in the repo only affects manual runs from the checkout directory.
The service runs as user `ibbq`, and the install script adds that user to the `bluetooth` group so the BLE adapter is accessible under systemd as well.
Discovered devices are persisted in `/var/lib/go-ibbq-mqtt/registry.json`; names and poll intervals are managed from the web UI, not from env vars.

If you prefer to install manually instead of using the script:

```bash
GOOS=linux GOARCH=arm GOARM=6 go build
sudo useradd -r -s /usr/sbin/nologin ibbq || true
sudo usermod -aG bluetooth ibbq
sudo install -d -m 0755 -o ibbq -g ibbq /var/lib/go-ibbq-mqtt
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
- `MQTT_TOPIC` — base MQTT topic prefix (e.g. `ibbq`)
- `WEB_PORT` — port for the web UI and API
- `LOGXI` — log verbosity (e.g. `*=INF`)

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

This repo includes a single `systemd` unit, `go-ibbq-mqtt.service`. One process scans for and manages all nearby iBBQ devices.

Install the binary, env file, and unit on the Pi:

```bash
./install.sh --install-go
```

Useful service commands:

```bash
sudo systemctl status go-ibbq-mqtt
sudo journalctl -u go-ibbq-mqtt -f
sudo systemctl restart go-ibbq-mqtt
```

The important bit is `systemctl enable`: that creates the boot-time symlink, so the service starts automatically on boot. `--now` also starts it immediately without waiting for a reboot.
