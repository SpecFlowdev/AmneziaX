# Changelog

All notable changes are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[semantic versioning](https://semver.org/spec/v2.0.0.html).

Tags with a pre-release identifier (`v0.2.0-beta.1`) publish an image under that
exact tag only — they never move `:latest`, so a beta cannot reach an install
that follows the stable channel.

## [Unreleased]

## [0.1.1] — 2026-08-02

### Added

- Xray JSON subscriptions — the array of complete client configurations that
  v2rayN, v2rayNG, Happ and Streisand accept — at `?format=json` and
  `/sub/<uuid>/json`. Every protocol and transport the panel can publish is
  checked against xray-core itself in the test suite.
- The subscription page links each format directly, for a client that wants a
  particular file or does not identify itself.
- A panel-wide **subscription format** setting decides what a client that does
  not identify itself receives — set it to Xray JSON and the subscription
  returns configurations instead of a base64 list of links. Clients that do
  announce themselves, Clash and sing-box, still get their own format, and
  `?format=` on the link overrides everything.

### Fixed

- The panel image took hours to build. Neither build stage was pinned to the
  build machine, so `buildx --platform linux/arm64` ran npm, Vite and every Go
  compile under QEMU emulation. Both stages are native now and Go
  cross-compiles to `$TARGETARCH`, which is what it is good at. CI builds the
  image on every push so a broken Dockerfile surfaces before a version is cut,
  not two hours into a release.
- `?format=json` was accepted and then quietly answered with base64: the format
  was declared valid but the renderer had no case for it.
- `/sub/<uuid>/json` returned the same account summary as `/sub/<uuid>/info`
  rather than anything a client could import.

### Changed

- A subscriber now gets one link instead of two. `/s/<uuid>` answers a browser
  with the subscription page and a client app with the configuration, so the
  same address can be opened, pasted or scanned. The panel had been showing both
  that page and the raw `/sub/<uuid>` endpoint, and opening the raw one in a
  browser downloaded a file rather than doing anything useful.
- `/sub/<uuid>` and its format-specific paths are unchanged, so links already
  sitting in someone's client keep working.

## [0.1.0] — 2026-08-01

First release: a working control plane for a fleet of Xray servers.

### Panel

- REST API and an embedded React UI served from one binary, plus a gRPC
  endpoint that node agents dial.
- Postgres storage with embedded migrations covering admins, config profiles,
  inbounds, nodes, hosts, squads, users, traffic history and an event log.
- JWT sessions with owner, administrator and read-only roles.
- Xray configuration profiles validated on save; inbounds are extracted by tag
  so hosts and squads keep pointing at the same identity across edits.
- Squads bundle inbounds and are assigned to users in one move.
- Hosts publish one inbound behind many domains, ports and SNIs, with
  `{{USERNAME}}`, `{{TAG}}`, `{{INBOUND}}` and `{{PROFILE}}` placeholders.
- Traffic quotas, expiry dates and daily, weekly or monthly resets. A user who
  crosses a limit is removed from the running configuration automatically.
- Per-node consumption multipliers and node-level traffic limits.
- White-label branding: panel name, tagline, logo and accent colour, applied to
  the sidebar, the sign-in screen and the subscription page.
- Infrastructure billing: provider, cost, currency and billing cycle per node,
  with monthly and yearly totals, cost per TB, a per-provider breakdown and
  upcoming payments on the dashboard.
- Device tracking against `hwidDeviceLimit` for clients that send a hardware id.
- API tokens for bots, billing systems and provisioning scripts.

### Nodes

- The agent dials the panel and holds one bidirectional stream open, so a node
  needs no inbound management port and works behind NAT.
- xray-core is supervised locally; a configuration the binary rejects is rolled
  back and reported rather than taking the node down.
- Host telemetry and per-user traffic, read from the xray stats API.
- Log tails and restart commands are driven from the panel.

### Subscriptions

- Base64, plain text, Clash/Mihomo YAML and sing-box JSON, selected from the
  client's `User-Agent` or pinned with `?format=`.
- A branded subscription page with a QR code.
- `subscription-userinfo` headers so clients show quota and expiry.

### Deployment

- `install-panel.sh` asks for a domain, installs Docker if needed, generates
  every secret and brings up Postgres, the panel and Caddy with a Let's Encrypt
  certificate. It falls back to building the images on the server when no
  published image matches.
- Caddy terminates TLS for the web UI on 443 and for the node control stream on
  9999; the panel itself binds no host port.
- `install-node.sh` installs xray-core and the agent and registers a systemd
  unit. The agent binary is downloaded from the panel, so a node needs nothing
  but `curl`.
- Russian and English throughout, in dark and light themes.

[Unreleased]: https://github.com/SpecFlowdev/AmneziaX/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.1.1
[0.1.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.1.0
