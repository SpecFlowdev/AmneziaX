<div align="center">

# AmneziaX

**A self-hosted control plane for Xray-core: one panel, many nodes.**

[Русская версия](README.ru.md) · [Architecture](docs/ARCHITECTURE.md) · [API reference](docs/API.md)

</div>

---

AmneziaX manages a fleet of Xray servers from a single web panel. You define
configuration profiles once, attach nodes to them, group inbounds into squads and
hand squads to users. The panel renders each node's exact `config.json`, pushes it
over a persistent gRPC stream and collects per-user traffic back — so adding a
user or revoking one takes effect in seconds, on every server at once.

It installs with one command and needs no Xray knowledge to get running: a fresh
install already contains a working VLESS + REALITY profile.

## Highlights

| | |
|---|---|
| **One-command install** | `install-panel.sh` brings up Postgres and the panel; the panel then hands you a one-liner for each node. |
| **Panel + node architecture** | Nodes dial *out* to the panel over gRPC, so they need no inbound management port and work behind NAT. |
| **Config profiles** | Full Xray documents, edited as JSON, validated before they ever reach a server. |
| **Squads** | Bundle inbounds and assign them to users in one move, across any number of nodes. |
| **Hosts** | Publish one inbound behind many domains, ports, SNIs or CDNs, with per-user labels. |
| **Live telemetry** | CPU, RAM, load, Xray uptime, online users and stacked traffic charts per node. |
| **Quotas that enforce themselves** | Traffic limits, expiry dates and daily/weekly/monthly resets; a user who hits their limit is removed from the running config automatically. |
| **Subscriptions** | Base64, plain-text and JSON endpoints plus a branded page with a QR code. |
| **Roles** | Owner, administrator and read-only accounts. |
| **Bilingual UI** | Russian and English, dark and light themes, in a deep crimson palette. |

## Quick start

On a fresh Debian, Ubuntu, Rocky or Alma server:

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/SpecFlowdev/AmneziaX/main/scripts/install-panel.sh)
```

The installer sets up Docker if needed, generates every secret, starts the stack
and prints your URL, username and password. Open the panel, sign in, and change
the password under **Settings**.

Then:

1. **Nodes → Add node.** Pick the starter profile and save. The panel shows a
   single command — run it on the server you want to turn into a node.
2. **Hosts → Add host.** Point it at your inbound and enter the address your
   clients will connect to. For REALITY, paste the public key and a short id
   (**Config profiles → Generate REALITY keys** produces both).
3. **Users → New user.** Give them the default squad and copy the subscription
   link.

### Ports

| Port | Who connects | Notes |
|---|---|---|
| `8080` | Administrators and subscribers | Put a TLS terminator (Caddy, nginx, Traefik) in front of it in production. |
| `9090` | Node agents | Must be reachable from every node. |
| inbound ports | VPN clients | Whatever your Xray inbounds listen on, on the nodes. |

### Putting the panel behind HTTPS

Terminate TLS with your usual reverse proxy and point it at `127.0.0.1:8080`,
then set `PANEL_PUBLIC_URL=https://panel.example.com` in `/opt/amneziax/.env`
and run `docker compose --env-file .env up -d`. Subscription links and node
install commands follow that URL.

## Configuration

The panel reads its settings from the environment (`/opt/amneziax/.env`).

| Variable | Default | Purpose |
|---|---|---|
| `DATABASE_URL` | — | Postgres connection string. **Required.** |
| `JWT_SECRET` | — | Signing key for admin sessions. **Required.** |
| `PANEL_HTTP_ADDR` | `:8080` | Listen address for the API and web UI. |
| `PANEL_GRPC_ADDR` | `:9090` | Listen address for node agents. |
| `PANEL_PUBLIC_URL` | `http://localhost:8080` | Public origin; used for subscription links and install commands. |
| `SUBSCRIPTION_PUBLIC_URL` | = `PANEL_PUBLIC_URL` | Override when subscriptions are served from another domain. |
| `PANEL_GRPC_PUBLIC_HOST` | derived | Host a node agent dials back on. |
| `PANEL_GRPC_PUBLIC_PORT` | `9090` | Port a node agent dials back on. |
| `PANEL_ADMIN_USERNAME` | `admin` | Owner account created on first boot. |
| `PANEL_ADMIN_PASSWORD` | generated | Owner password; printed to the log once when generated. |
| `JWT_TTL` | `24h` | Admin session lifetime. |
| `NODE_HEARTBEAT_INTERVAL` | `10s` | How often agents report health. |
| `NODE_USAGE_INTERVAL` | `30s` | How often agents report traffic. |
| `USAGE_RETENTION` | `2160h` | How long traffic history and events are kept. |
| `SUBSCRIPTION_TITLE` | `AmneziaX` | Profile name shown in client apps. |
| `SUBSCRIPTION_SUPPORT_URL` | — | Support link exposed to subscribers. |
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

```
                    ┌───────────────────────────────┐
   administrator ──▶│  Panel  (REST + web UI :8080) │
                    │         (gRPC       :9090)    │──▶ Postgres
   subscriber   ──▶ │  /sub/<uuid>                  │
                    └───────────────┬───────────────┘
                                    │ persistent bidirectional stream
                    ┌───────────────┴───────────────┐
                    │                               │
              ┌─────▼─────┐                   ┌─────▼─────┐
              │  Agent    │                   │  Agent    │
              │  xray-core│                   │  xray-core│
              └───────────┘                   └───────────┘
```

The agent dials the panel and keeps one stream open. The panel sends rendered
configurations and commands down it; the agent sends heartbeats, apply results,
traffic reports and log tails back up. Nothing polls, and a node behind NAT works
exactly like one with a public address.

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the data model and the sync
algorithm.

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
Serve the panel over HTTPS in production, and expose port `9090` only to your
nodes.

## License

See [LICENSE](LICENSE).
