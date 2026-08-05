# REST API

Base path `/api`. Everything is JSON. Errors come back as
`{"error": "human readable message"}` with a matching HTTP status.

## Authentication

`POST /api/auth/login` exchanges credentials for a bearer token.

```bash
curl -sX POST https://panel.example.com/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"…"}'
```

```json
{
  "token": "eyJhbGciOi…",
  "expiresAt": "2026-08-01T22:31:12Z",
  "admin": { "uuid": "…", "username": "admin", "role": "OWNER" }
}
```

Send it as `Authorization: Bearer <token>` on every other `/api` call. A `401`
means the session is gone; sign in again.

| Method | Path | Notes |
|---|---|---|
| `POST` | `/api/auth/login` | Public. `{username, password, code}`. |
| `GET` | `/api/auth/bootstrap-status` | Public. `{"initialized": bool}`. |
| `GET` | `/api/auth/me` | The signed-in administrator. |
| `POST` | `/api/auth/password` | `{currentPassword, newPassword}`. |

Sign-in is one call when the account has no second factor, and two when it has.
The first call omits `code`; if the account has two-factor the answer is `200`
with `{"totpRequired": true}` and **no token** — the password was right, but
that is not a session. Send the same credentials again with `code` set to a six
digit code or one recovery code.

When the panel requires two-factor and the account has none, the answer carries
a token *and* `{"enrolTotp": true}`: enrolment needs a session, so one is
issued, and the UI goes straight to setting the factor up.

Repeated failures are throttled per username **and** per source address —
either crossing five failures in fifteen minutes locks further attempts, first
for 30 seconds and doubling to a fifteen minute cap. A locked attempt gets
`429` and a `Retry-After` header, and is refused **before** the password is
checked, so knowing the right password does not bypass a lockout. A successful
sign-in clears both counters.

### Two-factor

Acts on the caller's own account, so no role is required — a read-only account
still secures its own sign-in.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/totp` | `{enabled, confirmedAt, recoveryCodesLeft, requiredByPanel}`. |
| `POST` | `/api/totp/start` | Stages a secret. `{secret, uri}` — `uri` is the `otpauth://` string a QR encodes. Not in force until confirmed. |
| `POST` | `/api/totp/confirm` | `{code}`. Turns it on and returns `{recoveryCodes}` — **the only time they are readable**. |
| `POST` | `/api/totp/disable` | `{password}`. Refused with `403` while the panel requires two-factor. |
| `POST` | `/api/totp/recovery-codes` | `{password}`. A fresh set; the previous ones stop working. |
| `POST` | `/api/admins/{uuid}/reset-totp` | Owner only. Clears someone else's factor when they lost both phone and codes; reveals nothing, so they must enrol again. |

TOTP is RFC 6238 — HMAC-SHA1, six digits, a thirty second step — which is what
authenticator apps implement. One step either side is accepted for clock drift.
An accepted code's time step is recorded, so the same code cannot be used twice
inside the window it stays valid; the code that confirms enrolment is burned the
same way, so a first sign-in needs the next one.

Recovery codes are single-use, stored only as digests, and matched ignoring case
and the separator.

### Roles

`OWNER` can do everything, including managing administrators. `ADMIN` can do
everything except that. `VIEWER` is read-only — any mutating request returns
`403`.

## System

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/health` | Public liveness probe. |
| `GET` | `/api/branding` | **Public.** Name, tagline, logo and accent, for the sign-in and subscription pages. |
| `GET` | `/api/settings` | Full panel settings. |
| `PUT` | `/api/settings` | Update branding, subscription title, support link, currency, and the warning thresholds below. |

`warnExpiryDays` and `warnQuotaPercent` control how early subscribers are warned.
Either set to `0` disables that warning, which is exactly how every install
behaved before they existed. Defaults are 3 days and 90%. The warnings are
emitted as the subscribable events `USER_EXPIRING_SOON` and
`USER_QUOTA_WARNING`, each sent once rather than once a minute.
| `GET` | `/api/system/attention` | What needs the operator right now: nodes offline, degraded or over quota; subscribers cut off, expiring or near their quota; node payments due within a week. Counts plus up to five names each. A pure read — looking at the dashboard never consumes a warning that has not been sent. |
| `GET` | `/api/system/spend` | Infrastructure cost: monthly and yearly totals, cost per TB, per-provider breakdown and upcoming payments. |
| `GET` | `/api/system/overview` | Counters plus panel build info. |
| `GET` | `/api/system/stats/traffic?days=7` | Total and per-node time series. |
| `GET` | `/api/system/stats/top-users?days=7&limit=10` | Biggest consumers. |
| `GET` | `/api/system/events?limit=100&kind=NODE_ERROR` | Audit log. |
| `GET` | `/api/search?q=…` | One query across users, nodes, hosts and squads. Returns `[{kind, uuid, label, hint}]`, at most five of each kind so a thousand matching users cannot push the one matching node off the list. An empty `q` returns `[]` rather than everything. |

## Config profiles

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/profiles` | All profiles with their extracted inbounds. |
| `GET` | `/api/profiles/inbounds?profileUuid=…` | Inbounds, optionally filtered. |
| `POST` | `/api/profiles` | `{name, kind, config}`. An omitted `kind` means `xray`. An empty `config` yields a working VLESS + REALITY starter. |
| `GET` | `/api/profiles/{uuid}` | One profile, including the nodes using it. |
| `PUT` | `/api/profiles/{uuid}` | Validates, saves, and pushes to every bound node. |
| `DELETE` | `/api/profiles/{uuid}` | `409` while any node still uses it. |
| `GET` | `/api/profiles/tools/starter?kind=…&domain=…` | `{kind, config}` — a document for that engine that already runs, generated and returned rather than stored. `domain` fills the certificate request of a `hysteria2` or `singbox` starter; it is ignored for `xray`, whose REALITY block borrows somebody else's name on the wire. |
| `POST` | `/api/profiles/tools/reality-keys` | `{privateKey, publicKey, shortIds}`. |
| `POST` | `/api/profiles/tools/wireguard-keys` | A fresh server pair, or — given `{privateKey}` — the public half of one you already have. A WireGuard host must name the server's *public* key, and deriving it beats keeping the two halves straight by hand. |

A profile carries a `kind` naming the engine its document is written for:
`xray` (the default, and what every profile written before this was), `singbox`
or `hysteria2`. The document is validated by that engine's rules, rendered by
its renderer, and run by it on the node. Everything else — squads granting
inbounds, hosts publishing them, users being revoked — works identically
whichever engine is behind it.

A hysteria2 document has no inbounds of its own, so the panel synthesises one
for it; that is what lets a squad grant it like anything else.

**Config profiles → New profile** picks the engine from a list and fills the box
with that engine's starter document, so a hysteria2 or sing-box profile is made
the same way an xray one is — nothing here needs the API.

For `kind: xray`, the `config` field is a full Xray document. Validation requires at least one
inbound and one outbound, a unique non-empty `tag` on every inbound, and rejects
the reserved tag `amneziax-api`.

## Nodes

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/nodes` | Includes live `isConnected` and telemetry. |
| `POST` | `/api/nodes` | Returns `{node, token, installCommand}`. **The token is shown once.** |
| `GET` | `/api/nodes/{uuid}` | |
| `PUT` | `/api/nodes/{uuid}` | Triggers a re-sync. |
| `DELETE` | `/api/nodes/{uuid}` | |
| `POST` | `/api/nodes/{uuid}/enable` · `/disable` | Disabling stops Xray on the node. |
| `POST` | `/api/nodes/{uuid}/restart` | Force-pushes the config and restarts Xray. |
| `POST` | `/api/nodes/{uuid}/sync` | Pushes only if the config actually changed. |
| `POST` | `/api/nodes/{uuid}/rotate-token` | New token and install command; the old one dies immediately. |
| `POST` | `/api/nodes/{uuid}/reset-traffic` | |
| `GET` | `/api/nodes/{uuid}/config` | The exact rendered document, clients included. |
| `GET` | `/api/nodes/{uuid}/logs?lines=200` | Fetched live from the agent. |
| `GET` | `/api/nodes/{uuid}/metrics?hours=24` | Heartbeat history: CPU, memory and load, one sample per minute. `hours` is capped at 30 days. |

Create payload:

```json
{
  "name": "nl-1",
  "address": "203.0.113.10",
  "countryCode": "NL",
  "configProfileUuid": "…",
  "activeInboundTags": ["vless-reality"],
  "consumptionMultiplier": 1,
  "trafficLimitBytes": 0,
  "trafficResetStrategy": "NO_RESET",
  "notifyPercent": 80,

  "provider": "Hetzner",
  "providerUrl": "https://console.hetzner.cloud",
  "costAmount": 12.5,
  "costCurrency": "EUR",
  "billingCycle": "MONTHLY",
  "nextPaymentAt": "2026-09-01T00:00:00Z",
  "billingNotes": "CX22",
  "tags": ["eu", "primary"]
}
```

Leave `activeInboundTags` empty to serve every inbound of the profile.
`billingCycle` is `NONE`, `MONTHLY`, `QUARTERLY` or `YEARLY`; costs are
normalised onto a monthly figure so nodes on different cycles can be summed.
A past-due `nextPaymentAt` rolls forward one cycle and leaves a
`NODE_PAYMENT_DUE` event behind.

## Hosts

| Method | Path |
|---|---|
| `GET` | `/api/hosts` |
| `POST` | `/api/hosts` |
| `GET` `PUT` `DELETE` | `/api/hosts/{uuid}` |
| `POST` | `/api/hosts/reorder` — `{order: [uuid, …]}` |

```json
{
  "inboundUuid": "…",
  "remark": "NL · {{USERNAME}}",
  "address": "nl.example.com",
  "port": 443,
  "security": "reality",
  "sni": "www.google.com",
  "fingerprint": "chrome",
  "publicKey": "…",
  "shortId": "ab12",
  "flow": "xtls-rprx-vision",
  "tags": ["premium"],
  "isDisabled": false
}
```

## Squads

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/squads` | With member counts and inbounds. |
| `POST` | `/api/squads` | `{name, info, inboundUuids}`. |
| `GET` `PUT` `DELETE` | `/api/squads/{uuid}` | |
| `POST` | `/api/squads/{uuid}/add-all-users` | Returns `{affected}`. |
| `POST` | `/api/squads/{uuid}/remove-all-users` | Returns `{affected}`. |

## Users

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/users` | Paginated; see the query parameters below. |
| `GET` | `/api/users/tags` | Distinct tags in use. |
| `POST` | `/api/users` | Credentials are generated server-side. |
| `GET` `PUT` `DELETE` | `/api/users/{uuid}` | |
| `POST` | `/api/users/{uuid}/enable` · `/disable` | |
| `POST` | `/api/users/{uuid}/reset-traffic` | Also clears a `LIMITED` status. |
| `POST` | `/api/users/{uuid}/revoke` | Rolls every credential and the subscription uuid. |
| `GET` | `/api/users/{uuid}/usage?days=30` | Hourly buckets. |
| `GET` | `/api/users/{uuid}/links` | Rendered connection links. |
| `GET` | `/api/users/{uuid}/devices` | Devices seen on this subscription. |
| `DELETE` | `/api/users/{uuid}/devices` | Forget every device. |
| `DELETE` | `/api/users/{uuid}/devices/{hwid}` | Forget one device. |
| `POST` | `/api/users/bulk` | `{uuids, action}` where action is `enable`, `disable`, `reset-traffic` or `delete`. |
| `POST` | `/api/users/bulk-create` | Create many at once — see below. |
| `GET` | `/api/users/export.csv?search=&status=&tag=` | The list as CSV, including each subscription link. Write-grade despite only reading, because those links are credentials. |

### Creating many users at once

Give either a `prefix` with a `count` (and an optional `start`), or an explicit
list of `names`. The list wins when both are present. Everything else in the
payload — status, traffic limit, reset cycle, squads, tag, device limit — is
applied to every user created.

```json
{ "prefix": "class-", "count": 12, "start": 1, "squadUuids": ["…"] }
```

Generated names are zero-padded to the width of the highest number, so
`class-01` sorts before `class-10` in every list an operator will read. At most
500 users per request — a guard against a typo in `count`, not a policy about
deployment size.

Names are created one at a time rather than in one transaction, so a single
duplicate costs that name and not the batch. The answer reports both halves:

```json
{
  "created": [ { "username": "class-01", "subscriptionUrl": "…" } ],
  "failed":  [ { "username": "class-02", "error": "a user with this name already exists" } ]
}
```

A pasted list is trimmed, blanks are dropped, and repeats are folded
case-insensitively before anything is created. The whole batch triggers one node
sync, not one per user.

List parameters: `search`, `status`, `squadUuid`, `tag`, `limit` (≤ 500),
`offset`, `sortBy` (`username`, `createdAt`, `expireAt`, `usedTraffic`, `status`,
`onlineAt`) and `desc`.

```json
{
  "items": [ { "uuid": "…", "username": "alice", "subscriptionUrl": "…" } ],
  "total": 128,
  "limit": 50,
  "offset": 0
}
```

Create payload:

```json
{
  "username": "alice",
  "status": "ACTIVE",
  "trafficLimitBytes": 107374182400,
  "trafficLimitStrategy": "MONTH",
  "expireAt": "2026-12-31T00:00:00Z",
  "squadUuids": ["…"],
  "tag": "vip",
  "email": "alice@example.com",
  "telegramId": 123456789,
  "hwidDeviceLimit": 3
}
```

## Notifications

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/notifications/events` | The event kinds a channel can subscribe to. |
| `GET` | `/api/notifications/channels` | Channels with their last delivery. **Secrets are never returned** — neither the webhook signing secret nor the Telegram bot token. |
| `POST` | `/api/notifications/channels` | `{name, kind, url, secret, botToken, chatId, events, isEnabled}`. `kind` is `WEBHOOK` or `TELEGRAM`. |
| `PUT` | `/api/notifications/channels/{uuid}` | A blank `secret` or `botToken` keeps the stored one; there is no way to read it back. |
| `DELETE` | `/api/notifications/channels/{uuid}` | |
| `POST` | `/api/notifications/channels/{uuid}/test` | Sends one delivery synchronously and returns what the endpoint answered. |
| `GET` | `/api/notifications/channels/{uuid}/deliveries?limit=50` | Every attempt, with status code, response excerpt and retry count. |

Webhook bodies are signed with HMAC-SHA256 over `timestamp + "." + body`. The
signature travels in `X-AmneziaX-Signature` and the timestamp it covers in
`X-AmneziaX-Timestamp` — verify both, and reject a timestamp far from now.

A `4xx` from the endpoint is treated as permanent and not retried; `5xx` and
`429` are retried with exponential backoff.

## Announcements

Shown to subscribers on their own page between `startsAt` and `endsAt`.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/announcements` | Including the ones not currently visible. |
| `POST` | `/api/announcements` | `{title, body, level, startsAt, endsAt, isEnabled}`. `level` is `INFO`, `WARNING` or `DANGER`. |
| `PUT` `DELETE` | `/api/announcements/{uuid}` | |

## Response rules

Pin a subscription format for a client the panel does not recognise on its own.
Rules are matched on the `User-Agent`, in priority order, and never override a
client that identifies itself.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/rules` | With each rule's hit counter and when it last matched. |
| `POST` | `/api/rules` | `{name, matchType, pattern, format, priority, isEnabled}`. `matchType` is `CONTAINS`, `PREFIX`, `EXACT` or `REGEX`. |
| `PUT` `DELETE` | `/api/rules/{uuid}` | |
| `GET` | `/api/rules/test?ua=…` | `{userAgent, format}` — what that client would actually be served. A probe never counts as a hit. |

## Inspectors

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/inspect/subscriptions?limit=200&user=…&failed=1` | Every subscription fetch: who, from what address, which client, which format was served and what the panel answered. `failed=1` keeps only `4xx`/`5xx`. Requests that resolved to nobody are kept, with the token that was tried. |
| `GET` | `/api/inspect/devices?limit=200&q=…` | Every known device across all subscribers. |
| `GET` | `/api/inspect/sessions?minutes=15&limit=200` | Who moved traffic recently and through which node, grouped from the nodes' usage reports. `minutes` is capped at 24 hours. |

Sessions are assembled from what the nodes already report. Individual
connections and the addresses they reach are not visible to the panel and are
not recorded anywhere.

## Backup

Owner only. A snapshot carries every credential in the deployment — node
enrolment tokens, API tokens, channel secrets and subscription uuids — so treat
the file exactly like the database itself.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/backup` | What a snapshot would contain: table names and row counts. |
| `GET` | `/api/backup/export` | The snapshot as one JSON file, read in a single consistent transaction. Traffic history is not included. |
| `POST` | `/api/backup/import` | **Replaces** the configuration rather than merging it, in one transaction. A snapshot from a different schema version is refused instead of being applied with columns dropped. |

## API tokens

Owner only. For bots, billing systems and provisioning scripts.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/tokens` | Never returns the secret, only a preview. |
| `POST` | `/api/tokens` | `{name, expiresAt}`. **The token is shown once.** |
| `DELETE` | `/api/tokens/{uuid}` | |

## Administrators

Owner only.

| Method | Path |
|---|---|
| `GET` | `/api/admins` |
| `POST` | `/api/admins` — `{username, password, role}` |
| `PUT` | `/api/admins/{uuid}` — `{username, password, role, isDisabled}` |
| `DELETE` | `/api/admins/{uuid}` |

`password` is optional on update; leave it out to keep the current one.

## Subscriptions

Public. The token is either the user's `subscriptionUuid` or their `shortUuid`.

| Method | Path | Returns |
|---|---|---|
| `GET` | `/sub/{token}` | The format the client understands — see below. |
| `GET` | `/sub/{token}/links` | The plain link list. |
| `GET` | `/sub/{token}/clash` | Clash / Mihomo YAML. |
| `GET` | `/sub/{token}/singbox` | sing-box JSON. |
| `GET` | `/sub/{token}/wireguard` | The subscriber's WireGuard `.conf`. There is no URI scheme for WireGuard, so the file is the only thing to hand over. |
| `GET` | `/sub/{token}/json` | A full Xray client config — inbounds, outbounds, routing and DNS — ready to drop into an Xray-based app. |
| `GET` | `/sub/{token}/info` | Quota, expiry, visible announcements and the links, used by the subscription page. |

`/sub/{token}` decides what to return in this order: an explicit `?format=`,
then the client's `User-Agent` if it identifies itself (Clash, Mihomo, Stash and
FlClash get YAML; sing-box, Hiddify and Karing get JSON), then the first
matching response rule, then the panel's configured default, and finally base64.
`?format=base64|plain|clash|singbox|json|wireguard` overrides everything.

### WireGuard

Xray-core serves WireGuard, so it needs no second binary on a node. Unlike the
other protocols it identifies a peer by a key pair rather than a shared secret,
so every subscriber gets their own Curve25519 pair and a fixed address inside
the tunnel. The node receives only public keys; the private half stays in the
panel because the panel is what hands out the `.conf`.

Publish a WireGuard inbound as a host with the server's **public** key in
`publicKey` and the UDP port in `port`. One `.conf` is one tunnel: when several
WireGuard hosts are published the first enabled one is served, rather than
emitting two `[Peer]` blocks both claiming `0.0.0.0/0` — that is a file whose
behaviour depends on which client opens it.

The Clash and sing-box documents can be replaced with your own from Settings;
the panel splices the servers each subscriber is entitled to into `{{PROXIES}}`,
`{{NAMES}}`, `{{OUTBOUNDS}}`, `{{TAGS}}` and `{{TITLE}}`. A template that uses
none of them is served verbatim, so the subscriber's servers will not appear in
it.

Responses carry `profile-title`, `profile-update-interval` and
`subscription-userinfo` headers so clients can show quota and expiry. A disabled
user gets `403`; an unknown token gets `404`.

If the client sends `X-Hwid` (or `X-Device-Id`), the device is recorded against
the user and counted towards `hwidDeviceLimit`. A new device beyond the limit
gets `403`; a device already known always passes, and a request without the
header is never blocked.

`GET /s/{token}` (no `/sub`) serves the human-readable page with a QR code when
a browser asks for it, and the subscription itself to everything else — so the
same link can be pasted into an app or opened by hand. What the page shows is
configurable: the format buttons and the raw connection links can each be
hidden, and a hidden link is not sent at all, not merely styled away.

Every fetch of `/sub/{token}` and `/s/{token}` is recorded and readable through
`/api/inspect/subscriptions`, including the ones that resolved to nobody.

## Installers

| Method | Path | Returns |
|---|---|---|
| `GET` | `/install-node.sh` | The node installer. Public — it carries no secrets. |
| `GET` | `/install-panel.sh` | The panel installer. |
| `GET` | `/dist/amneziax-node-linux-{amd64,arm64}` | The agent binary, when the deployment bundles it. This is what makes a node install need only `curl`. |

## Node control stream

Node agents do not use the REST API. They open a bidirectional gRPC stream at
`node.v1.NodeControl/Connect`, authenticated by the enrolment token in the first
message. Behind the bundled Caddy the stream has its own TLS port — `9999` by
default — which routes `/node.v1.NodeControl/*` to the panel's gRPC listener and
answers `404` to anything else. See [ARCHITECTURE.md](ARCHITECTURE.md).
