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
| `POST` | `/api/auth/login` | Public. |
| `GET` | `/api/auth/bootstrap-status` | Public. `{"initialized": bool}`. |
| `GET` | `/api/auth/me` | The signed-in administrator. |
| `POST` | `/api/auth/password` | `{currentPassword, newPassword}`. |

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
| `PUT` | `/api/settings` | Update branding, subscription title, support link and currency. |
| `GET` | `/api/system/spend` | Infrastructure cost: monthly and yearly totals, cost per TB, per-provider breakdown and upcoming payments. |
| `GET` | `/api/system/overview` | Counters plus panel build info. |
| `GET` | `/api/system/stats/traffic?days=7` | Total and per-node time series. |
| `GET` | `/api/system/stats/top-users?days=7&limit=10` | Biggest consumers. |
| `GET` | `/api/system/events?limit=100&kind=NODE_ERROR` | Audit log. |

## Config profiles

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api/profiles` | All profiles with their extracted inbounds. |
| `GET` | `/api/profiles/inbounds?profileUuid=…` | Inbounds, optionally filtered. |
| `POST` | `/api/profiles` | `{name, config}`. An empty `config` yields a working VLESS + REALITY starter. |
| `GET` | `/api/profiles/{uuid}` | One profile, including the nodes using it. |
| `PUT` | `/api/profiles/{uuid}` | Validates, saves, and pushes to every bound node. |
| `DELETE` | `/api/profiles/{uuid}` | `409` while any node still uses it. |
| `POST` | `/api/profiles/tools/reality-keys` | `{privateKey, publicKey, shortIds}`. |

The `config` field is a full Xray document. Validation requires at least one
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
| `GET` | `/sub/{token}/json` | A full Xray client config — inbounds, outbounds, routing and DNS — ready to drop into an Xray-based app. |
| `GET` | `/sub/{token}/info` | Quota, expiry, visible announcements and the links, used by the subscription page. |

`/sub/{token}` decides what to return in this order: an explicit `?format=`,
then the client's `User-Agent` if it identifies itself (Clash, Mihomo, Stash and
FlClash get YAML; sing-box, Hiddify and Karing get JSON), then the first
matching response rule, then the panel's configured default, and finally base64.
`?format=base64|plain|clash|singbox|json` overrides everything.

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
