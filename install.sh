#!/usr/bin/env bash

set -euo pipefail

SERVICE_USER="${SERVICE_USER:-ibbq}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/go-ibbq-mqtt}"
ENV_PATH="${ENV_PATH:-/etc/default/go-ibbq-mqtt}"
SERVICE_PATH="${SERVICE_PATH:-/etc/systemd/system/go-ibbq-mqtt.service}"
TEMPLATE_PATH="${TEMPLATE_PATH:-/etc/systemd/system/go-ibbq-mqtt@.service}"

OVERWRITE_ENV=0
ENABLE_SERVICE=1
INSTALL_GO=0
MIN_GO_VERSION="${MIN_GO_VERSION:-1.21}"
GO_INSTALL_VERSION="${GO_INSTALL_VERSION:-1.22.2}"
GO_INSTALL_VERSION_ARMV6="${GO_INSTALL_VERSION_ARMV6:-1.21.13}"

usage() {
	cat <<'EOF'
Usage: ./install.sh [--overwrite-env] [--skip-enable] [--install-go]

Installs the binary and systemd units for go-ibbq-mqtt.

Options:
  --overwrite-env  Replace an existing /etc/default/go-ibbq-mqtt
  --skip-enable    Install files but do not enable/start the service
  --install-go     Install official Go to /usr/local/go if missing or too old
EOF
}

version_ge() {
	[[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | tail -n1)" == "$1" ]]
}

install_go() {
	local arch goarch go_version tarball url tmp_tarball

	arch="$(uname -m)"
	case "$arch" in
	x86_64)
		goarch="amd64"
		go_version="$GO_INSTALL_VERSION"
		;;
	aarch64)
		goarch="arm64"
		go_version="$GO_INSTALL_VERSION"
		;;
	armv6l|armv7l)
		goarch="armv6l"
		go_version="$GO_INSTALL_VERSION_ARMV6"
		;;
	*)
		echo "Unsupported architecture for automatic Go install: $arch" >&2
		exit 1
		;;
	esac

	if ! command -v curl >/dev/null 2>&1; then
		echo "curl is required for --install-go. Install it first: sudo apt install -y curl" >&2
		exit 1
	fi

	if ! command -v tar >/dev/null 2>&1; then
		echo "tar is required for --install-go. Install it first: sudo apt install -y tar" >&2
		exit 1
	fi

	tarball="go${go_version}.linux-${goarch}.tar.gz"
	url="https://go.dev/dl/${tarball}"
	tmp_tarball="/tmp/${tarball}"

	echo "Downloading Go ${go_version} for ${goarch}"
	curl -fsSL "$url" -o "$tmp_tarball"

	echo "Installing Go to /usr/local/go"
	sudo rm -rf /usr/local/go
	sudo tar -C /usr/local -xzf "$tmp_tarball"
	rm -f "$tmp_tarball"

	sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
	sudo ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

	export PATH="/usr/local/go/bin:$PATH"
	echo "Installed $(go version)"
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
	--install-go)
		INSTALL_GO=1
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

if ! command -v go >/dev/null 2>&1; then
	if [[ "$INSTALL_GO" -eq 1 ]]; then
		install_go
	else
		echo "Go is not installed. Install Go ${MIN_GO_VERSION}+ first." >&2
		echo "Suggested command on Debian/Raspberry Pi OS: sudo apt update && sudo apt install -y golang" >&2
		echo "Or rerun this script with --install-go to install official Go ${GO_INSTALL_VERSION}." >&2
		exit 1
	fi
fi

GO_VERSION_RAW="$(go version)"
GO_VERSION="$(awk '{print $3}' <<<"$GO_VERSION_RAW" | sed 's/^go//')"
if [[ -z "$GO_VERSION" ]] || ! version_ge "$GO_VERSION" "$MIN_GO_VERSION"; then
	if [[ "$INSTALL_GO" -eq 1 ]]; then
		install_go
		GO_VERSION_RAW="$(go version)"
		GO_VERSION="$(awk '{print $3}' <<<"$GO_VERSION_RAW" | sed 's/^go//')"
	fi
fi

if [[ -z "$GO_VERSION" ]] || ! version_ge "$GO_VERSION" "$MIN_GO_VERSION"; then
	echo "Go ${MIN_GO_VERSION}+ is required, found: ${GO_VERSION_RAW}" >&2
	echo "Suggested command on Debian/Raspberry Pi OS: sudo apt update && sudo apt install -y golang" >&2
	echo "Or rerun this script with --install-go to install official Go ${GO_INSTALL_VERSION}." >&2
	exit 1
fi

echo "Building go-ibbq-mqtt binary"
GOOS=linux go build

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
