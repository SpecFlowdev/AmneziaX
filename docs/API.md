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
| `GET` | `/sub/{token}/json` | Structured info plus links. |
| `GET` | `/sub/{token}/info` | Same payload, used by the subscription page. |

`/sub/{token}` picks its encoding from the client's `User-Agent`: Clash, Mihomo,
Stash and FlClash get YAML; sing-box, Hiddify and Karing get JSON; everything
else gets the base64 list. `?format=base64|plain|clash|singbox|json` overrides
the guess.

Responses carry `profile-title`, `profile-update-interval` and
`subscription-userinfo` headers so clients can show quota and expiry. A disabled
user gets `403`; an unknown token gets `404`.

If the client sends `X-Hwid` (or `X-Device-Id`), the device is recorded against
the user and counted towards `hwidDeviceLimit`. A new device beyond the limit
gets `403`; a device already known always passes, and a request without the
header is never blocked.

`GET /s/{token}` (no `/sub`) serves the human-readable page with a QR code.

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
