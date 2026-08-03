# Changelog

All notable changes are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[semantic versioning](https://semver.org/spec/v2.0.0.html).

Tags with a pre-release identifier (`v0.2.0-beta.1`) publish an image under that
exact tag only — they never move `:latest`, so a beta cannot reach an install
that follows the stable channel.

## [Unreleased]

## [0.3.0] — 2026-08-03

### Added

- **Backup and restore.** One file holds the whole configuration — settings,
  admins, profiles, nodes, hosts, squads, users, notification channels,
  announcements — and can be loaded back. History (traffic, events, heartbeat
  samples, delivery receipts) is deliberately left out: it is large, it is
  pruned on a schedule anyway, and it rebuilds itself.
  Export runs in one repeatable-read transaction, so the snapshot is a single
  consistent moment rather than a smear. Restore runs in one transaction and
  replaces rather than merges, because a half-merged panel is worse than
  either state. Each column's Postgres type is recorded alongside the data,
  since JSON cannot tell a `text[]` from a `jsonb` array once both are a list
  of strings — and a snapshot from a different schema version is refused
  outright rather than loaded with columns silently dropped.
  Owner-only: the file contains every subscription link, node token and
  password hash in the deployment, and the UI says so.
- Node load over the last 24 hours is drawn on each node card. CPU and memory
  share one 0–100% axis, the x axis is real time so an outage leaves a gap,
  and the two series differ in dash pattern as well as hue.

### Changed

- The interface is quieter. Cards had a border, a shadow and a translucent
  tint all saying the same thing, so the page read as boxes inside boxes;
  the hairline stays and the rest is gone. Primary buttons, the logo mark and
  the traffic meter lose their gradients and coloured shadows for one flat
  fill. The active navigation item is a rule down its left edge instead of a
  tinted pill competing with real buttons. Also removed: the crimson corner
  wash, the backdrop blurs behind the sidebar and topbar, and the 700-weight
  wide-tracked section labels. Corner radii drop from 14px to 10px.
- Both READMEs describe the current panel again — they had not moved since the
  first release — with screenshots retaken against the reworked interface.

## [0.2.0] — 2026-08-03

### Added

- **Notifications.** Events now leave the panel instead of only being recorded.
  Webhooks are signed with HMAC-SHA256 over the timestamp and the exact bytes
  sent, so a receiver can tell a real delivery from anyone who learned the URL;
  Telegram messages escape HTML, because parse_mode turns a username into
  markup. Retries distinguish "understood and refused" from "try later" — a 4xx
  is written off after one attempt, a 5xx or 429 gets three with exponential
  backoff — and every attempt lands in a delivery log, so "the webhook never
  arrived" stops being unanswerable. Secrets are write-only.
- **Announcements.** A scheduled notice shown to subscribers on their own page,
  so an operator can warn about maintenance without messaging everyone. A
  window that has not opened yet is stored but never served.
- Node heartbeat samples are kept. The nodes table only held the latest
  reading, which says a node is at 90% CPU but not whether it has been there
  for an hour. Samples are bucketed by minute and pruned after 30 days.

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

[Unreleased]: https://github.com/SpecFlowdev/AmneziaX/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.3.0
[0.2.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.2.0
[0.1.1]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.1.1
[0.1.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.1.0
