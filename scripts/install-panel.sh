#!/usr/bin/env bash
#
# AmneziaX panel installer.
#
#   bash <(curl -fsSL https://raw.githubusercontent.com/SpecFlowdev/AmneziaX/main/scripts/install-panel.sh)
#
# The panel always runs behind Caddy, which obtains a Let's Encrypt certificate
# for your domain and terminates TLS for both the web UI and the node control
# stream. Only ports 80 and 443 are ever exposed.
#
# Re-running the script is safe: existing secrets are kept and only the images
# and the reverse-proxy configuration are refreshed.

set -euo pipefail

REPO_RAW="${AMNEZIAX_RAW:-https://raw.githubusercontent.com/SpecFlowdev/AmneziaX/main}"
INSTALL_DIR="${AMNEZIAX_DIR:-/opt/amneziax}"
DOMAIN="${AMNEZIAX_DOMAIN:-}"
ACME_EMAIL="${AMNEZIAX_ACME_EMAIL:-}"
BUILD_LOCAL=0
ASSUME_YES=0
SKIP_DNS_CHECK=0

RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'; BLUE=$'\033[0;36m'; BOLD=$'\033[1m'; NC=$'\033[0m'
info() { printf '%s==>%s %s\n' "$GREEN" "$NC" "$*"; }
warn() { printf '%s !! %s %s\n' "$YELLOW" "$NC" "$*"; }
die()  { printf '%serror:%s %s\n' "$RED" "$NC" "$*" >&2; exit 1; }

usage() {
  cat <<EOF
Usage: install-panel.sh [options]

  --domain DOMAIN     domain the panel is served on, e.g. panel.example.com
  --email EMAIL       address for Let's Encrypt expiry notices (optional)
  --dir PATH          install directory (default: $INSTALL_DIR)
  --build             build the images from this checkout instead of pulling
  --skip-dns-check    do not verify that the domain resolves to this server
  -y, --yes           accept the defaults and do not ask anything
  -h, --help          show this help

The domain is required. Point an A (or AAAA) record at this server before
running the installer — Let's Encrypt validates over HTTP on port 80.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --domain) DOMAIN="$2"; shift 2 ;;
    --email) ACME_EMAIL="$2"; shift 2 ;;
    --dir) INSTALL_DIR="$2"; shift 2 ;;
    --build) BUILD_LOCAL=1; shift ;;
    --skip-dns-check) SKIP_DNS_CHECK=1; shift ;;
    -y|--yes) ASSUME_YES=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1 (try --help)" ;;
  esac
done

[[ $EUID -eq 0 ]] || die "run this as root (sudo bash ...)"

# ---------------------------------------------------------------- dependencies

pkg_install() {
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y -qq "$@"
  elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q "$@"
  elif command -v yum >/dev/null 2>&1; then
    yum install -y -q "$@"
  else
    die "install these manually and re-run: $*"
  fi
}

missing=()
for tool in curl openssl; do
  command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
done
[[ ${#missing[@]} -eq 0 ]] || { info "installing ${missing[*]}"; pkg_install "${missing[@]}"; }

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  info "installing Docker"
  curl -fsSL https://get.docker.com | sh || die "Docker installation failed"
  systemctl enable --now docker >/dev/null 2>&1 || true
  docker compose version >/dev/null 2>&1 || die "the docker compose plugin is missing"
fi

# ---------------------------------------------------------------- the domain

valid_domain() {
  [[ "$1" =~ ^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$ ]]
}

if [[ -z "$DOMAIN" ]]; then
  [[ -t 0 ]] || die "no domain given and no terminal to ask on — pass --domain"
  printf '\n%sAmneziaX is served over HTTPS, so it needs a domain.%s\n' "$BOLD" "$NC"
  printf 'Point an A record at this server first, then enter it below.\n\n'
  while :; do
    read -r -p "  Panel domain (e.g. panel.example.com): " DOMAIN
    DOMAIN="${DOMAIN,,}"
    DOMAIN="${DOMAIN#http://}"; DOMAIN="${DOMAIN#https://}"; DOMAIN="${DOMAIN%%/*}"
    if valid_domain "$DOMAIN"; then break; fi
    warn "that does not look like a domain name — try again"
  done
fi

DOMAIN="${DOMAIN,,}"
DOMAIN="${DOMAIN#http://}"; DOMAIN="${DOMAIN#https://}"; DOMAIN="${DOMAIN%%/*}"
valid_domain "$DOMAIN" || die "invalid domain: $DOMAIN"
[[ "$DOMAIN" != "localhost" ]] || die "Let's Encrypt cannot issue a certificate for localhost"

if [[ -z "$ACME_EMAIL" && $ASSUME_YES -eq 0 && -t 0 ]]; then
  read -r -p "  Email for certificate expiry notices (optional, Enter to skip): " ACME_EMAIL
fi

# ---------------------------------------------------------------- pre-flight

server_ip="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || true)"

if [[ $SKIP_DNS_CHECK -eq 0 ]]; then
  resolved="$(getent ahostsv4 "$DOMAIN" 2>/dev/null | awk 'NR==1{print $1}')"
  if [[ -z "$resolved" ]]; then
    warn "$DOMAIN does not resolve yet — Let's Encrypt will fail until it does"
  elif [[ -n "$server_ip" && "$resolved" != "$server_ip" ]]; then
    warn "$DOMAIN resolves to $resolved, but this server is $server_ip"
    warn "if that is a proxy or a load balancer this is fine; otherwise fix DNS first"
  else
    info "$DOMAIN resolves to this server"
  fi
fi

for port in 80 443; do
  if command -v ss >/dev/null 2>&1 && ss -tlnH "sport = :$port" 2>/dev/null | grep -q .; then
    holder="$(ss -tlnpH "sport = :$port" 2>/dev/null | head -1 | sed 's/.*users:((//;s/).*//' || true)"
    # Our own Caddy from a previous run is expected; anything else is a clash.
    if ! docker ps --format '{{.Image}}' 2>/dev/null | grep -q '^caddy'; then
      warn "port $port is already in use ${holder:+by $holder} — stop that service, Caddy needs it"
    fi
  fi
done

# ---------------------------------------------------------------- configuration

mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"

FRESH_INSTALL=0
if [[ -f .env ]]; then
  info "keeping the existing secrets in $INSTALL_DIR/.env"
  # shellcheck disable=SC1091
  set -a; source .env; set +a
else
  FRESH_INSTALL=1
  POSTGRES_PASSWORD="$(openssl rand -hex 24)"
  JWT_SECRET="$(openssl rand -hex 32)"
  PANEL_ADMIN_PASSWORD="$(openssl rand -base64 15 | tr -d '/+=' | cut -c1-16)"
  PANEL_ADMIN_USERNAME="admin"
fi

cat > .env <<EOF
# Generated by install-panel.sh — keep this file private.
AMNEZIAX_DOMAIN=${DOMAIN}
ACME_EMAIL=${ACME_EMAIL}

POSTGRES_USER=amneziax
POSTGRES_DB=amneziax
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}

JWT_SECRET=${JWT_SECRET}

PANEL_ADMIN_USERNAME=${PANEL_ADMIN_USERNAME:-admin}
PANEL_ADMIN_PASSWORD=${PANEL_ADMIN_PASSWORD}

SUBSCRIPTION_TITLE=AmneziaX
LOG_LEVEL=info
EOF
chmod 600 .env

info "writing the Caddy configuration"
if [[ $BUILD_LOCAL -eq 1 && -f deploy/Caddyfile ]]; then
  cp deploy/Caddyfile Caddyfile.site
else
  curl -fsSL -o Caddyfile.site "${REPO_RAW}/deploy/Caddyfile" \
    || die "could not download the Caddy configuration"
fi

# Caddy rejects an empty `email` argument, so the global block only appears
# when an address was actually given.
{
  if [[ -n "$ACME_EMAIL" ]]; then
    printf '{\n\temail %s\n}\n\n' "$ACME_EMAIL"
  fi
  cat Caddyfile.site
} > Caddyfile
rm -f Caddyfile.site

if [[ $BUILD_LOCAL -eq 0 ]]; then
  info "fetching the compose file"
  curl -fsSL -o docker-compose.yml "${REPO_RAW}/deploy/docker-compose.yml" \
    || die "could not download the compose file"
  # Without a local checkout there is no build context, so drop the build stanza.
  sed -i '/^    build:/,/^      dockerfile:.*$/d' docker-compose.yml
  COMPOSE=(docker compose --env-file .env)
else
  COMPOSE=(docker compose -f deploy/docker-compose.yml --env-file .env)
  export CADDYFILE="${INSTALL_DIR}/Caddyfile"
fi

# ---------------------------------------------------------------- launch

if [[ $ASSUME_YES -eq 0 && -t 0 ]]; then
  printf '\n%sAbout to install AmneziaX%s\n' "$BOLD" "$NC"
  printf '  directory : %s\n' "$INSTALL_DIR"
  printf '  panel URL : %shttps://%s%s\n' "$BLUE" "$DOMAIN" "$NC"
  printf '  nodes     : %s:443 (TLS, through Caddy)\n' "$DOMAIN"
  printf '  ports     : 80 and 443 only\n\n'
  read -r -p 'Continue? [Y/n] ' answer
  [[ -z "$answer" || "$answer" =~ ^[Yy]$ ]] || die "aborted"
fi

info "starting the stack"
if [[ $BUILD_LOCAL -eq 1 ]]; then
  "${COMPOSE[@]}" up -d --build
else
  "${COMPOSE[@]}" pull
  "${COMPOSE[@]}" up -d
fi

info "waiting for the panel"
ready=0
for _ in $(seq 1 60); do
  if "${COMPOSE[@]}" exec -T panel wget -qO- http://127.0.0.1:8080/api/health >/dev/null 2>&1 ||
     curl -fsS --max-time 3 "https://${DOMAIN}/api/health" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 2
done
[[ $ready -eq 1 ]] || {
  warn "the panel did not come up — check: cd ${INSTALL_DIR} && docker compose logs -f panel"
  exit 1
}

info "waiting for the certificate"
cert_ok=0
for _ in $(seq 1 45); do
  if curl -fsS --max-time 4 "https://${DOMAIN}/api/health" >/dev/null 2>&1; then
    cert_ok=1
    break
  fi
  sleep 2
done

echo
if [[ $cert_ok -eq 1 ]]; then
  printf '%s%sAmneziaX is running at https://%s%s\n' "$GREEN" "$BOLD" "$DOMAIN" "$NC"
else
  printf '%s%sAmneziaX is running, but the certificate is not ready yet.%s\n' "$YELLOW" "$BOLD" "$NC"
  printf 'Caddy keeps retrying. Watch it with:\n  cd %s && docker compose logs -f caddy\n' "$INSTALL_DIR"
  printf 'The usual cause is DNS not pointing here yet, or port 80 being blocked.\n'
fi

cat <<EOF

  URL       : https://${DOMAIN}
  Username  : ${PANEL_ADMIN_USERNAME:-admin}
EOF
[[ $FRESH_INSTALL -eq 1 ]] && printf '  Password  : %s\n' "$PANEL_ADMIN_PASSWORD"
cat <<EOF

Secrets live in ${INSTALL_DIR}/.env — back that file up.

Next steps:
  1. Sign in and change the password under Settings.
  2. Create a node; the panel gives you a one-line install command for it.
     Nodes reach the panel at ${DOMAIN}:443 over TLS, so no extra port
     needs opening on this server.
  3. Add a host for your inbound, then create users.

Manage the stack with:
  cd ${INSTALL_DIR} && docker compose --env-file .env [ps|logs -f|restart|down]

EOF
