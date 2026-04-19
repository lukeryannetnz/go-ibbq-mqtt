#!/usr/bin/env bash

set -euo pipefail

SERVICE_USER="${SERVICE_USER:-ibbq}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/go-ibbq-mqtt}"
ENV_PATH="${ENV_PATH:-/etc/default/go-ibbq-mqtt}"
SERVICE_PATH="${SERVICE_PATH:-/etc/systemd/system/go-ibbq-mqtt.service}"
TEMPLATE_PATH="${TEMPLATE_PATH:-/etc/systemd/system/go-ibbq-mqtt@.service}"

OVERWRITE_ENV=0
ENABLE_SERVICE=1

usage() {
	cat <<'EOF'
Usage: ./install.sh [--overwrite-env] [--skip-enable]

Installs the binary and systemd units for go-ibbq-mqtt.

Options:
  --overwrite-env  Replace an existing /etc/default/go-ibbq-mqtt
  --skip-enable    Install files but do not enable/start the service
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--overwrite-env)
		OVERWRITE_ENV=1
		shift
		;;
	--skip-enable)
		ENABLE_SERVICE=0
		shift
		;;
	-h|--help)
		usage
		exit 0
		;;
	*)
		echo "Unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [[ ! -f go-ibbq-mqtt.service ]] || [[ ! -f go-ibbq-mqtt@.service ]] || [[ ! -f .env.example ]]; then
	echo "Run this script from the repository root." >&2
	exit 1
fi

if [[ ! -x ./go-ibbq-mqtt ]]; then
	echo "Building go-ibbq-mqtt binary"
	GOOS=linux go build
fi

if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
	echo "Creating service user $SERVICE_USER"
	sudo useradd -r -s /usr/sbin/nologin "$SERVICE_USER"
fi

echo "Adding $SERVICE_USER to bluetooth group"
sudo usermod -aG bluetooth "$SERVICE_USER"

echo "Installing binary to $BIN_PATH"
sudo install -m 0755 ./go-ibbq-mqtt "$BIN_PATH"

if [[ ! -f "$ENV_PATH" ]] || [[ "$OVERWRITE_ENV" -eq 1 ]]; then
	echo "Installing env file to $ENV_PATH"
	sudo install -m 0644 .env.example "$ENV_PATH"
else
	echo "Keeping existing env file at $ENV_PATH"
fi

echo "Installing systemd units"
sudo install -m 0644 go-ibbq-mqtt.service "$SERVICE_PATH"
sudo install -m 0644 go-ibbq-mqtt@.service "$TEMPLATE_PATH"

echo "Reloading systemd"
sudo systemctl daemon-reload

if [[ "$ENABLE_SERVICE" -eq 1 ]]; then
	echo "Enabling and starting go-ibbq-mqtt.service"
	sudo systemctl enable --now go-ibbq-mqtt
	sudo systemctl status --no-pager go-ibbq-mqtt
else
	echo "Skipping systemctl enable/start"
fi

echo
echo "Edit $ENV_PATH to set MQTT_SERVER, MQTT_TOPIC, DEVICE_MAC, DEVICE_NAME, and WEB_PORT if needed."
