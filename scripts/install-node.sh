#!/usr/bin/env bash
#
# AmneziaX node installer. The panel shows the exact command to paste, e.g.
#
#   bash <(curl -fsSL https://panel.example.com/install-node.sh) \
#        --panel panel.example.com:9999 --uuid <node-uuid> --token <token> --tls
#
# It installs xray-core and the agent, registers a systemd unit and starts it.
# The agent only makes outbound connections, so the node needs no open ports
# beyond the ones its inbounds listen on.

set -euo pipefail

PANEL_ADDR=""
NODE_UUID=""
NODE_TOKEN=""
INSECURE="true"
SERVER_NAME=""
PANEL_URL=""
XRAY_VERSION="${XRAY_VERSION:-v25.1.30}"
AGENT_VERSION="${AGENT_VERSION:-latest}"
RELEASE_BASE="${AMNEZIAX_RELEASE_BASE:-https://github.com/SpecFlowdev/AmneziaX/releases}"

INSTALL_DIR=/var/lib/amneziax-node
BIN_DIR=/usr/local/bin

RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'; BOLD=$'\033[1m'; NC=$'\033[0m'
info() { printf '%s==>%s %s\n' "$GREEN" "$NC" "$*"; }
warn() { printf '%s!! %s %s\n' "$YELLOW" "$NC" "$*"; }
die()  { printf '%serror:%s %s\n' "$RED" "$NC" "$*" >&2; exit 1; }

usage() {
  cat <<EOF
Usage: install-node.sh --panel HOST:PORT --uuid UUID --token TOKEN [options]

  --panel HOST:PORT   the panel's node endpoint (its domain and 9999 behind Caddy)
  --uuid UUID         node uuid shown by the panel
  --token TOKEN       one-time enrolment token
  --tls               dial the panel over TLS — required behind Caddy
  --panel-url URL     panel origin to download the agent binary from
  --server-name NAME  TLS server name, when the panel sits behind a proxy
  --xray-version V    xray-core release to install (default: $XRAY_VERSION)
  -h, --help          show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --panel) PANEL_ADDR="$2"; shift 2 ;;
    --uuid) NODE_UUID="$2"; shift 2 ;;
    --token) NODE_TOKEN="$2"; shift 2 ;;
    --tls) INSECURE="false"; shift ;;
    --panel-url) PANEL_URL="${2%/}"; shift 2 ;;
    --server-name) SERVER_NAME="$2"; INSECURE="false"; shift 2 ;;
    --xray-version) XRAY_VERSION="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

[[ $EUID -eq 0 ]] || die "run this as root (sudo bash ...)"
[[ -n "$PANEL_ADDR" ]] || die "--panel is required"
[[ -n "$NODE_UUID" ]] || die "--uuid is required"
[[ -n "$NODE_TOKEN" ]] || die "--token is required"

case "$(uname -m)" in
  x86_64|amd64) GOARCH=amd64; XRAY_ASSET=Xray-linux-64.zip ;;
  aarch64|arm64) GOARCH=arm64; XRAY_ASSET=Xray-linux-arm64-v8a.zip ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

# ---------------------------------------------------------------- dependencies

# apt on a fresh cloud image is usually busy with unattended-upgrades, so the
# lock has to be waited for rather than treated as a failure.
wait_for_apt() {
  local waited=0
  while fuser /var/lib/dpkg/lock-frontend /var/lib/dpkg/lock /var/lib/apt/lists/lock \
        /var/cache/apt/archives/lock >/dev/null 2>&1; do
    if [[ $waited -eq 0 ]]; then
      info "waiting for another package manager to finish (unattended-upgrades?)"
    fi
    if (( waited >= 300 )); then
      warn "the package lock is still held after 5 minutes"
      return 1
    fi
    sleep 5
    waited=$((waited + 5))
  done
  return 0
}

pkg_install() {
  if command -v apt-get >/dev/null 2>&1; then
    wait_for_apt || true
    export DEBIAN_FRONTEND=noninteractive
    # DPkg::Lock::Timeout makes apt itself queue instead of erroring out; the
    # explicit wait above covers older releases that do not honour it.
    apt-get -o DPkg::Lock::Timeout=300 update -qq \
      && apt-get -o DPkg::Lock::Timeout=300 install -y -qq "$@"
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q "$@"
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q "$@"
  elif command -v apk >/dev/null 2>&1; then
    apk add --no-cache "$@"
  else
    die "install these manually and re-run: $*"
  fi
}

command -v curl >/dev/null 2>&1 || { info "installing curl"; pkg_install curl \
  || die "curl is required and could not be installed"; }

# unzip is only needed to unpack the xray release, and a locked package manager
# should not stop the install when python3 can do the same job.
unpack_zip() {
  local archive="$1" dest="$2"
  mkdir -p "$dest"
  if command -v unzip >/dev/null 2>&1; then
    unzip -oq "$archive" -d "$dest"
  elif command -v python3 >/dev/null 2>&1; then
    python3 - "$archive" "$dest" <<'PYEOF'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1]) as z:
    z.extractall(sys.argv[2])
PYEOF
  elif command -v busybox >/dev/null 2>&1; then
    busybox unzip -oq "$archive" -d "$dest"
  else
    return 1
  fi
}

if ! command -v unzip >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1; then
  info "installing unzip"
  pkg_install unzip || warn "could not install unzip — will try other unpackers"
fi

command -v systemctl >/dev/null 2>&1 || die "this installer needs systemd"

# ---------------------------------------------------------------- xray-core

if [[ -x "$BIN_DIR/xray" ]] && "$BIN_DIR/xray" version >/dev/null 2>&1; then
  info "xray-core already installed: $("$BIN_DIR/xray" version | head -1)"
else
  info "installing xray-core $XRAY_VERSION"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  curl -fsSL -o "$tmp/xray.zip" \
    "https://github.com/XTLS/Xray-core/releases/download/${XRAY_VERSION}/${XRAY_ASSET}" \
    || die "could not download xray-core"
  unpack_zip "$tmp/xray.zip" "$tmp/xray" || die "could not unpack the xray archive — install unzip or python3 and re-run"
  install -m 0755 "$tmp/xray/xray" "$BIN_DIR/xray"
  mkdir -p /usr/local/share/xray
  install -m 0644 "$tmp/xray/geoip.dat" "$tmp/xray/geosite.dat" /usr/local/share/xray/
fi

# ---------------------------------------------------------------- agent

info "installing the AmneziaX agent"

# Sources are tried in order of how little the node has to have installed:
# the panel itself ships the matching binary, then published releases, and
# only then a build from source.
agent_sources=()
[[ -n "$PANEL_URL" ]] && agent_sources+=("${PANEL_URL}/dist/amneziax-node-linux-${GOARCH}")
if [[ "$AGENT_VERSION" == "latest" ]]; then
  agent_sources+=("${RELEASE_BASE}/latest/download/amneziax-node-linux-${GOARCH}")
else
  agent_sources+=("${RELEASE_BASE}/download/${AGENT_VERSION}/amneziax-node-linux-${GOARCH}")
fi

downloaded=0
for url in "${agent_sources[@]}"; do
  if curl -fsSL --retry 2 -o "$BIN_DIR/amneziax-node.new" "$url" \
     && [[ -s "$BIN_DIR/amneziax-node.new" ]]; then
    info "downloaded the agent from ${url%%/dist/*}"
    downloaded=1
    break
  fi
done

if [[ $downloaded -eq 0 ]]; then
  warn "could not download a prebuilt agent — building from source"
  command -v git >/dev/null 2>&1 || pkg_install git || die "git is required to build from source"
  command -v go >/dev/null 2>&1 || die "install Go and re-run, or check that the panel is reachable"
  src="$(mktemp -d)"
  git clone --depth 1 https://github.com/SpecFlowdev/AmneziaX "$src/AmneziaX" >/dev/null 2>&1 \
    || die "could not clone the repository"
  (cd "$src/AmneziaX" && CGO_ENABLED=0 go build -trimpath -o "$BIN_DIR/amneziax-node.new" ./cmd/node) \
    || die "build failed"
  rm -rf "$src"
fi

chmod 0755 "$BIN_DIR/amneziax-node.new"
"$BIN_DIR/amneziax-node.new" --help >/dev/null 2>&1 || true
mv "$BIN_DIR/amneziax-node.new" "$BIN_DIR/amneziax-node"

# ---------------------------------------------------------------- service

id amneziax >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin amneziax
mkdir -p "$INSTALL_DIR"
chown -R amneziax:amneziax "$INSTALL_DIR"

cat > /etc/amneziax-node.env <<EOF
PANEL_GRPC_ADDR=${PANEL_ADDR}
NODE_UUID=${NODE_UUID}
NODE_TOKEN=${NODE_TOKEN}
PANEL_GRPC_INSECURE=${INSECURE}
PANEL_GRPC_SERVER_NAME=${SERVER_NAME}
XRAY_BINARY=${BIN_DIR}/xray
XRAY_WORKDIR=${INSTALL_DIR}
XRAY_API_ADDR=127.0.0.1:10085
XRAY_LOCATION_ASSET=/usr/local/share/xray
LOG_LEVEL=info
EOF
chmod 600 /etc/amneziax-node.env

cat > /etc/systemd/system/amneziax-node.service <<EOF
[Unit]
Description=AmneziaX node agent
Documentation=https://github.com/SpecFlowdev/AmneziaX
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=amneziax
Group=amneziax
EnvironmentFile=/etc/amneziax-node.env
ExecStart=${BIN_DIR}/amneziax-node
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=5
LimitNOFILE=1048576
# Binding to 443 and other privileged ports without running the whole agent as root.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=full
ProtectHome=true
PrivateTmp=true
ReadWritePaths=${INSTALL_DIR}

[Install]
WantedBy=multi-user.target
EOF

info "starting amneziax-node"
systemctl daemon-reload
systemctl enable --now amneziax-node

sleep 3
if systemctl is-active --quiet amneziax-node; then
  cat <<EOF

${GREEN}${BOLD}The node is installed and connecting to ${PANEL_ADDR}.${NC}

  status : systemctl status amneziax-node
  logs   : journalctl -u amneziax-node -f
  config : /etc/amneziax-node.env

It should appear as online in the panel within a few seconds.

EOF
else
  warn "the agent is not running — inspect it with: journalctl -u amneziax-node -n 60 --no-pager"
  exit 1
fi
