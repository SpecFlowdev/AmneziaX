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
  "notifyPercent": 80
}
```

Leave `activeInboundTags` empty to serve every inbound of the profile.

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
| `GET` | `/sub/{token}` | Base64 link list — what client apps import. |
| `GET` | `/sub/{token}/links` | The same list as plain text. |
| `GET` | `/sub/{token}/json` | Structured info plus links. |
| `GET` | `/sub/{token}/info` | Same payload, used by the subscription page. |

Responses carry `profile-title`, `profile-update-interval` and
`subscription-userinfo` headers so clients can show quota and expiry. A disabled
user gets `403`; an unknown token gets `404`.

`GET /s/{token}` (no `/sub`) serves the human-readable page with a QR code.

## Installers

| Method | Path | Returns |
|---|---|---|
| `GET` | `/install-node.sh` | The node installer. Public — it carries no secrets. |
| `GET` | `/install-panel.sh` | The panel installer. |

## Node control stream

Node agents do not use the REST API. They open a bidirectional gRPC stream at
`node.v1.NodeControl/Connect`, authenticated by the enrolment token in the first
message. Behind the bundled Caddy that stream shares port 443 with the API: the
proxy routes `/node.v1.NodeControl/*` to the panel's gRPC listener and
everything else to the web UI. See [ARCHITECTURE.md](ARCHITECTURE.md).
