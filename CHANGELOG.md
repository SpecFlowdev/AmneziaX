# Changelog

All notable changes are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[semantic versioning](https://semver.org/spec/v2.0.0.html).

Tags with a pre-release identifier (`v0.2.0-beta.1`) publish an image under that
exact tag only — they never move `:latest`, so a beta cannot reach an install
that follows the stable channel.

## [Unreleased]

## [0.8.0] — 2026-08-05

### Added

- **One-tap import into client apps.** A subscriber was handed a link and left
  to work out what to do with it — paste it where, in which app, on a phone.
  The page now offers the apps themselves: Happ, v2rayNG, Streisand, Hiddify,
  Shadowrocket, sing-box, Clash and V2Box, each as that app's own URL scheme
  with the subscription inside it. The list leads with what runs on the device
  the page was opened from, since a Shadowrocket button on Android is noise, and
  the rest stay one click away because user agents are a hint and not a fact.
  Nothing here can detect whether an app is installed — the browser just hands
  the URL over and nothing visible happens if nobody claims it — so the copy
  link and the QR stay exactly where they were, and the hint says plainly what
  to do when a button appears to do nothing.
- **Command palette.** `Ctrl`/`⌘` + `K` from anywhere in the panel. Every page
  is a few letters away, and the same box finds a user, node, host or squad by
  name. It is one endpoint across all four tables rather than four requests,
  because the palette queries on every keystroke and four round trips per
  keystroke is how a search box comes to feel slow. Answers that arrive after a
  newer keystroke are discarded, so the list never flips back to what was typed
  two letters ago.
- **Bulk user creation.** Handing access to a class, an office or a reseller's
  customers meant filling the same form thirty times. Now it takes either a
  prefix and a count or a pasted list — the two ways the names already exist.
  Generated numbers are zero-padded to the width of the largest, so `class-01`
  sorts before `class-10` everywhere an operator will read them, and the exact
  names are previewed before anything is created. Names are created one at a
  time rather than in one transaction, so a single duplicate costs that name and
  not the batch; the answer reports what was made and what was not, with the
  reason. A pasted list is trimmed and de-duplicated first. The whole batch
  triggers one node sync rather than one per user, and the new names and their
  links come back as a file.
- **CSV export of users**, with the current filters applied, including each
  subscription link. That link is a credential, which is why the export is not
  something a read-only account can pull.


## [0.7.0] — 2026-08-04

### Added

- **Two-factor authentication for administrators.** The panel holds every node
  token, subscription link and password hash in the deployment, and a password
  was the only thing in front of it. TOTP is computed on the standard library
  rather than pulled in as a dependency, and checked against the RFC 6238 test
  vectors so it matches what an authenticator app will produce rather than
  merely matching itself.
  Enrolment is two steps — the secret is staged, and only a code the app
  actually produced turns it on — because a secret written straight to the
  account locks out anyone who scanned a QR into an app that never worked.
  An accepted code's time step is recorded, so the same code cannot be used
  twice inside the thirty seconds it stays valid; without that a code read over
  someone's shoulder is reusable, and a second factor that can be replayed is a
  slower password. The code that confirms enrolment is burned the same way.
  Ten single-use recovery codes are shown once and stored only as digests, and
  are forgiving about case and the separator, because they get retyped from
  paper. Turning two-factor off, or regenerating the codes, asks for the
  password again: a session left open on an unlocked machine should not be
  enough. An owner can require it panel-wide, and can reset it for someone who
  lost both their phone and their codes — clearing it rather than revealing
  anything.
- **Sign-in throttling.** The form had a uniform 300ms delay and nothing else,
  which is not an obstacle to a script. Failures are now counted against the
  username *and* the source address, because counting only usernames lets one
  attacker walk a list and counting only addresses lets a botnet spread a single
  guess. Either crossing five failures in fifteen minutes locks further attempts
  for 30 seconds, doubling to a fifteen minute cap. The lock is checked before
  the password is, so the right password does not walk through it, and the
  response carries `Retry-After` rather than leaving a client guessing. A
  successful sign-in clears both counters, so a typo does not follow anyone
  around. Lockouts are their own event kind, subscribable like any other.

### Changed

- **The docs caught up with the code.** `API.md` had been left behind three
  releases: notifications, announcements, response rules, the inspectors,
  backup and node metrics were all reachable and none of them were written
  down, and `/sub/{token}/json` was still described as "structured info plus
  links" when it now returns a full Xray client configuration. `ARCHITECTURE.md`
  likewise stopped at traffic accounting. Both now cover every endpoint the
  router serves — checked by walking the route table rather than by reading —
  and the architecture notes state plainly what sessions cannot show and why.
- **Screenshots retaken.** The inspectors picture predated the sessions tab and
  showed a two-tab screen that no longer exists, and the subscription format,
  page options and templates had no picture at all. Both READMEs now show
  Settings alongside the inspectors.
- Reworded the template hint in English; the Russian one already read clearly.

## [0.6.0] — 2026-08-03

### Added

- **Sessions.** A third inspector tab showing who has carried traffic recently
  and through which node. Assembled from the usage the nodes already report
  rather than from a per-connection feed, so it cannot show individual
  connections or their addresses — that would need the agent to report them,
  and the tab says so instead of implying a precision it does not have.
- **Subscription templates.** Clash and sing-box are whole configuration
  documents, and the panel rendered both from a template baked into the binary
  — right for most deployments, and impossible to change for an operator who
  needs their own rules, DNS or proxy groups. A custom document can now be
  supplied for either, with the panel splicing in the part only it knows: the
  servers a subscriber is entitled to. Empty keeps the built-in rendering, so
  nothing changes until someone opts in. A template naming no placeholder is
  served verbatim — a deliberate escape hatch for pinning one fixed
  configuration, and the hint says plainly that a subscriber's own servers will
  not appear in it.
- **Subscription page options.** The subscriber's page was fixed for everyone:
  QR, link, format buttons and the raw `vless://` connection strings. Handing
  those to a non-technical subscriber invites them to paste the wrong one, and
  an operator supporting such users had no way to hide them. Two toggles in
  Settings now control the format buttons and the connection links. Hiding the
  links removes them from the payload rather than from the markup — a value
  that is not sent cannot be read out of the page source.

## [0.5.0] — 2026-08-03

### Added

- **Response rules.** The panel knew a built-in list of clients and one global
  default for everything else, so an operator whose users run something it has
  never heard of could only change that default for everybody at once. A rule
  matches a substring of the User-Agent — not a regex, because operators write
  these and a bad regex fails at request time on a path that must not fail —
  and pins a format. Rules sit below the built-in detection, never above it,
  and above the global default. Each counts its hits, and the screen has a
  probe that answers what a given client would actually be served, including
  when a built-in match or the default wins instead.

### Fixed

- The rule hit counter read three for a single request: the format is resolved
  twice per request, once to answer and once to log, and the operator's probe
  counted as well. A counter that inflates is worse than none, since its only
  job is to tell you whether a rule ever fires. Counting now happens once, on
  the answering path.

## [0.4.0] — 2026-08-03

### Added

- **Inspectors.** Two questions that span every subscriber at once and were not
  answerable from a single user's page. *Subscription requests* logs every
  fetch — who, which client, which format was served, from what address, and
  what the panel answered — including requests that resolved to nobody, which
  is where a revoked link still being polled shows up. *Devices* lists every
  known hardware id across all subscribers, searchable. Both are pruned after
  30 days.

### Fixed

- A release was published before the image it referred to existed. The job that
  creates the GitHub release ran in parallel with the image build and finished
  about three minutes earlier, so for those minutes `v0.3.0` was visible while
  `:latest` still pointed at `0.2.0` — and anyone installing in that window got
  the previous version with nothing to indicate it. The release now waits for
  the image.

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

[Unreleased]: https://github.com/SpecFlowdev/AmneziaX/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.6.0
[0.5.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.5.0
[0.4.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.4.0
[0.3.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.3.0
[0.2.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.2.0
[0.1.1]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.1.1
[0.1.0]: https://github.com/SpecFlowdev/AmneziaX/releases/tag/v0.1.0
