<div align="center">

<img src="docs/assets/header.svg" alt="AmneziaX" width="100%">

[![CI](https://github.com/SpecFlowdev/AmneziaX/actions/workflows/ci.yml/badge.svg)](https://github.com/SpecFlowdev/AmneziaX/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/SpecFlowdev/AmneziaX?include_prereleases&sort=semver&color=e11d48)](https://github.com/SpecFlowdev/AmneziaX/releases)
[![License](https://img.shields.io/github/license/SpecFlowdev/AmneziaX?color=e11d48)](LICENSE)

[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=black)](https://react.dev)
[![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![Docker](https://img.shields.io/badge/Docker-compose-2496ED?logo=docker&logoColor=white)](https://docs.docker.com/compose/)
[![Caddy](https://img.shields.io/badge/Caddy-2-1F88C0?logo=caddy&logoColor=white)](https://caddyserver.com)
[![gRPC](https://img.shields.io/badge/gRPC-streaming-244c5a?logo=grpc&logoColor=white)](https://grpc.io)
[![Xray](https://img.shields.io/badge/Xray--core-VLESS%20%C2%B7%20REALITY-e11d48)](https://github.com/XTLS/Xray-core)

**[Русская версия](README.ru.md)** · [Architecture](docs/ARCHITECTURE.md) · [API reference](docs/API.md) · [Changelog](CHANGELOG.md)

</div>

---

AmneziaX manages a fleet of Xray servers from a single web panel. You define
configuration profiles once, attach nodes to them, group inbounds into squads and
hand squads to users. The panel renders each node's exact `config.json`, pushes it
over a persistent gRPC stream and collects per-user traffic back — so adding a
user or revoking one takes effect in seconds, on every server at once.

It installs with one command and needs no Xray knowledge to get running: a fresh
install already contains a working VLESS + REALITY profile.

<div align="center">
  <img src="docs/assets/dashboard.png" alt="The AmneziaX dashboard" width="100%">
</div>

## Highlights

| | |
|---|---|
| **One-command install** | `install-panel.sh` asks for your domain, brings up Postgres, the panel and Caddy, and gets a certificate; the panel then hands you a one-liner for each node. |
| **HTTPS by default** | Caddy terminates TLS for the panel on 443 and for the node control stream on 9999, under one certificate. The panel binds no host port of its own. |
| **Panel + node architecture** | Nodes dial *out* to the panel over gRPC, so they need no inbound management port and work behind NAT. |
| **Config profiles** | Full Xray documents, edited as JSON, validated before they ever reach a server. |
| **Squads** | Bundle inbounds and assign them to users in one move, across any number of nodes. |
| **Hosts** | Publish one inbound behind many domains, ports, SNIs or CDNs, with per-user labels. |
| **Live telemetry** | CPU, RAM, load, Xray uptime, online users and stacked traffic charts per node. |
| **Quotas that enforce themselves** | Traffic limits, expiry dates and daily/weekly/monthly resets; a user who hits their limit is removed from the running config automatically. |
| **Subscriptions** | One link per subscriber that a browser opens as a page and an app imports as a config. |
| **Roles** | Owner, administrator and read-only accounts. |
| **Bilingual UI** | Russian and English, dark and light themes, in a deep crimson palette. |
| **White-label branding** | Set the panel's name, logo and accent colour from Settings; they apply to the sidebar, the sign-in screen and the subscription page. |
| **Infrastructure billing** | Record what each node costs, from which provider and on what cycle. The dashboard totals monthly spend, cost per TB and what is due next. |
| **Client-aware subscriptions** | Xray JSON, Clash/Mihomo YAML, sing-box JSON, plain and base64, chosen from the client's User-Agent or pinned with `?format=`. |
| **Device limits** | Clients that send a hardware id are tracked and capped per user; devices are listed and can be forgotten individually. |
| **API tokens** | Scoped tokens for bots, billing systems and provisioning scripts. |

## Quick start

Point a DNS **A record** at your server first — the certificate is issued over
HTTP on port 80, so the domain has to resolve before you start.

On a fresh Debian, Ubuntu, Rocky or Alma server:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/SpecFlowdev/AmneziaX/main/scripts/install-panel.sh)
```

It asks for your domain, then installs Docker if needed, generates every secret,
starts Postgres, the panel and Caddy, and waits for the certificate. When no
published image matches the current code it builds one on the server instead of
failing, which adds a few minutes to a first install. You can also pass
everything up front:

```bash
bash <(curl -fsSL .../install-panel.sh) --domain panel.example.com --email you@example.com -y
```

When it finishes it prints your URL, username and password. Open the panel, sign
in, and change the password under **Settings**.

Then:

1. **Nodes → Add node.** Pick the starter profile and save. The panel shows a
   single command — run it on the server you want to turn into a node. The agent
   binary comes from the panel itself, so the node needs nothing but `curl`.
2. **Hosts → Add host.** Point it at your inbound and enter the address your
   clients will connect to. For REALITY, paste the public key and a short id
   (**Config profiles → Generate REALITY keys** produces both).
3. **Users → New user.** Give them the default squad and copy the subscription
   link.

### Ports

Three ports are exposed on the panel server, all of them owned by Caddy:

| Port | Who connects | Notes |
|---|---|---|
| `80` | Let's Encrypt, and browsers being redirected | Required for certificate issuance and renewal. |
| `443` | Administrators and subscribers | The web UI, the API and the subscription endpoints. |
| `9999` | Node agents | The gRPC control stream, over TLS with the same certificate. Change it with `--node-port`. |
| inbound ports | VPN clients | Whatever your Xray inbounds listen on — on the *nodes*, not here. |

The panel itself never binds a host port: `8080` and `9090` exist only inside the
Docker network, and Caddy is the only way in.

### The node port

Agents dial `your-domain:9999` over ordinary TLS. Caddy serves only the node
service there — a gRPC call arrives as `POST /node.v1.NodeControl/Connect`, and
anything that is not that path gets a `404`, so a port scan finds nothing useful.
Requests are forwarded to the panel's gRPC listener over h2c with no timeout and
no buffering, because an agent holds one stream open for its whole lifetime.

### Using your own reverse proxy instead

If you already run nginx or Traefik, drop the `caddy` service from
`docker-compose.yml`, publish `8080` and `9090` yourself, and set
`PANEL_PUBLIC_URL`, `PANEL_GRPC_PUBLIC_HOST`, `PANEL_GRPC_PUBLIC_PORT` and
`PANEL_GRPC_PUBLIC_TLS` to match how you expose them. Those four values are what
the panel prints in node install commands and subscription links.

## Configuration

The panel reads its settings from the environment (`/opt/amneziax/.env`).

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | — | Postgres connection string. **Required.** |
| `JWT_SECRET` | — | Signing key for admin sessions. **Required.** |
| `PANEL_HTTP_ADDR` | `:8080` | Listen address for the API and web UI. |
| `PANEL_GRPC_ADDR` | `:9090` | Listen address for node agents. |
| `AMNEZIAX_DOMAIN` | — | Domain Caddy serves and requests a certificate for. **Required.** |
| `AMNEZIAX_NODE_PORT` | `9999` | Port Caddy exposes the node control stream on. |
| `PANEL_PUBLIC_URL` | `http://localhost:8080` | Public origin; used for subscription links and install commands. Set to `https://$AMNEZIAX_DOMAIN` by the installer. |
| `SUBSCRIPTION_PUBLIC_URL` | = `PANEL_PUBLIC_URL` | Override when subscriptions are served from another domain. |
| `PANEL_GRPC_PUBLIC_HOST` | derived | Host a node agent dials back on. |
| `PANEL_GRPC_PUBLIC_PORT` | `9090` | Port a node agent dials back on. `9999` behind Caddy. |
| `PANEL_GRPC_PUBLIC_TLS` | `false` | Whether generated install commands tell the agent to dial over TLS. |
| `PANEL_ADMIN_USERNAME` | `admin` | Owner account created on first boot. |
| `PANEL_ADMIN_PASSWORD` | generated | Owner password; printed to the log once when generated. |
| `JWT_TTL` | `24h` | Admin session lifetime. |
| `NODE_HEARTBEAT_INTERVAL` | `10s` | How often agents report health. |
| `NODE_USAGE_INTERVAL` | `30s` | How often agents report traffic. |
| `USAGE_RETENTION` | `2160h` | How long traffic history and events are kept. |
| `SUBSCRIPTION_TITLE` | `AmneziaX` | Profile name shown in client apps. |
| `SUBSCRIPTION_SUPPORT_URL` | — | Support link exposed to subscribers. |
| `AGENT_DIST_DIR` | `/usr/local/share/amneziax/dist` | Prebuilt agent binaries the panel serves at `/dist`, so nodes install without a toolchain. |
| `CORS_ORIGINS` | `*` | Comma-separated allowed origins. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |

The node agent (`/etc/amneziax-node.env`):

| Variable | Default | Purpose |
|---|---|---|
| `PANEL_GRPC_ADDR` | — | `host:port` of the panel. **Required.** |
| `NODE_UUID` | — | Node identity from the panel. **Required.** |
| `NODE_TOKEN` | — | Enrolment token. **Required.** |
| `PANEL_GRPC_INSECURE` | `true` | Set to `false` to dial the panel over TLS. |
| `PANEL_GRPC_SERVER_NAME` | — | TLS server name when the panel is behind a proxy. |
| `XRAY_BINARY` | `/usr/local/bin/xray` | Path to xray-core. |
| `XRAY_WORKDIR` | `/var/lib/amneziax-node` | Where the rendered config lives. |
| `XRAY_API_ADDR` | `127.0.0.1:10085` | Local stats API the agent reads counters from. |

## How it fits together

<div align="center">
  <img src="docs/assets/architecture.svg" alt="AmneziaX architecture" width="100%">
</div>

The agent dials the panel and keeps one stream open. The panel sends rendered
configurations and commands down it; the agent sends heartbeats, apply results,
traffic reports and log tails back up. Nothing polls, and a node behind NAT works
exactly like one with a public address.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the data model and the sync
algorithm.

## A look around

| Nodes | Users |
|---|---|
| <img src="docs/assets/nodes.png" alt="The nodes page" width="100%"> | <img src="docs/assets/users.png" alt="The users page" width="100%"> |

Live telemetry per node, one-line install commands, rendered configuration and
log access on the left; quotas, squads, devices and subscription links on the
right.

## Building from source

```bash
git clone https://github.com/SpecFlowdev/AmneziaX
cd AmneziaX
make ui      # compile the SPA into internal/webui/dist
make build   # bin/amneziax-panel and bin/amneziax-node
make test
```

Requirements: Go 1.24+, Node 20+, and `protoc` only if you change `proto/`.

Run it locally against a Postgres of your choice:

```bash
DATABASE_URL='postgres://user:pass@127.0.0.1:5432/amneziax?sslmode=disable' \
JWT_SECRET=dev-secret \
PANEL_ADMIN_PASSWORD=devpassword \
./bin/amneziax-panel
```

During UI work, `cd frontend && npm run dev` proxies `/api` and `/sub` to
`127.0.0.1:8080`.

## Operating notes

- **Back up Postgres.** It holds every user, credential and configuration.
- **Rotate a node token** from the node card if a server is ever compromised; the
  old token stops working immediately.
- **Revoke a user** to roll all of their credentials at once — the old
  subscription link and any imported config stop working right away.
- **Editing a profile** re-renders and pushes to every node bound to it. A
  configuration that Xray rejects is rolled back on the node, and the node
  reports the error instead of going dark.

## Security

The panel signs admin sessions with `JWT_SECRET`, stores admin passwords with
bcrypt and node tokens as SHA-256 digests. Subscription URLs are unguessable
UUIDs and are the only credential a subscriber needs, so treat them as secrets.

Everything reaches the panel through Caddy over TLS, including the node control
stream, and the panel binds no host port of its own. That leaves 80, 443 and the
node port as the only attack surface on the panel server — and the node port
answers nothing except the control service.

## Contributing

Bug reports, ideas and pull requests are welcome — see
[CONTRIBUTING.md](CONTRIBUTING.md) for how to get a development environment
running and what CI expects. Security problems go through
[SECURITY.md](SECURITY.md), not a public issue.

## License

MIT — see [LICENSE](LICENSE).
