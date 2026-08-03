# Architecture

AmneziaX is two programs, one database and a reverse proxy.

- **`cmd/panel`** — the control plane. Serves the REST API and the embedded web
  UI on one port, and the node control stream on another. Owns all state.
- **`cmd/node`** — the agent. Supervises one `xray-core` process and does what
  the panel tells it. Holds no state of its own beyond the config on disk.
- **Postgres** — every user, node, host, profile, squad and traffic sample.
- **Caddy** — the only process bound to a host port. It obtains and renews the
  certificate and terminates TLS for everything, including the node stream.

## The edge

The panel publishes nothing itself; `8080` and `9090` are reachable only inside
the Docker network. Caddy owns every exposed port and serves two sites under one
certificate:

```
your-domain:443    ──▶  http://panel:8080   API, web UI, subscriptions
your-domain:9999   ──▶  h2c://panel:9090    node control stream
```

The node site only routes `/node.v1.NodeControl/*` and answers `404` to
everything else. That path prefix is reliable because a gRPC request path is
always `/<package>.<service>/<method>`, so the node service is unambiguous and
nothing else on that port is reachable.

Agents dial the node port with ordinary TLS; gRPC negotiates HTTP/2 through
ALPN, Caddy re-encodes it as h2c to the backend, and the stream stays open for
the lifetime of the agent because that route is configured with no read or write
timeout and no response buffering.

The upshot is certificate-backed transport security for node traffic without a
private CA, and a panel server whose entire attack surface is 80, 443 and one
port that speaks only gRPC.

## Repository layout

```
proto/node/v1/node.proto        the panel <-> node contract
gen/go/node/v1/                 generated Go bindings (make proto)
cmd/panel, cmd/node             entry points
internal/domain                 entities shared by every layer
internal/config                 environment-driven settings
internal/auth                   JWT issuing, bcrypt, node tokens
internal/storage/postgres       repositories and embedded migrations
internal/xray                   config parsing, validation and rendering
internal/subscription           connection links and client formats
internal/notify                 webhook and Telegram delivery
internal/hub                    live agent sessions and the sync engine
internal/httpapi                REST handlers and middleware
internal/webui                  serves the compiled SPA from the binary
frontend/                       React + TypeScript SPA
scripts/                        installers, embedded into the panel binary
deploy/                         Dockerfiles and the compose stack
```

## Data model

```
ConfigProfile ─┬─< ConfigProfileInbound ─┬─< Host
               │                          └─< SquadInbound >─ Squad ─< UserSquad >─ User
               └─< Node
```

**ConfigProfile** is a complete Xray configuration document. On save the panel
validates it and extracts one `ConfigProfileInbound` row per inbound, matched by
tag. That row is the stable identity everything else points at, so renaming a
profile or editing unrelated parts of the document never breaks a host or a
squad.

**Node** is a server. It is bound to one profile and to a list of inbound tags it
actually serves — leave the list empty and it serves all of them. A node also
carries its own traffic counter, quota, reset cycle and consumption multiplier.

**Host** is what a client connects to. It points at an inbound but stores its own
address, port, SNI, REALITY keys and so on, so one inbound can be published
behind several domains or CDNs. A host's label supports `{{USERNAME}}`,
`{{TAG}}`, `{{INBOUND}}` and `{{PROFILE}}`.

**Squad** bundles inbounds. Users are attached to squads, never to inbounds
directly, which is what makes bulk access changes cheap.

**User** owns the credentials: a VLESS uuid, a trojan password, a shadowsocks
password, and a subscription uuid that acts as their personal link. Revoking a
user rolls all of them at once.

## The control stream

The agent dials the panel and opens a single bidirectional gRPC stream
(`NodeControl.Connect`). This direction matters: a node needs no inbound
management port and works unchanged behind NAT.

```
agent                                            panel
  │ Hello{node_uuid, token, version, system}  ──▶ │  verify SHA-256(token)
  │ ◀── HelloAck{accepted, intervals}             │  mark node connected
  │                                               │
  │ ◀── ApplyConfig{xray_config, hash}            │  rendered for this node
  │ ApplyResult{ok, hash, error}              ──▶ │  store hash / record failure
  │                                               │
  │ Heartbeat{cpu, ram, load, xray state}     ──▶ │  every 10s
  │ UsageReport{per-user and per-inbound}     ──▶ │  every 30s
  │                                               │
  │ ◀── Command{restart | stop | fetch_logs}      │  operator actions
  │ LogChunk{lines}                           ──▶ │
```

The panel authenticates the agent by comparing a SHA-256 digest of the enrolment
token in constant time. Tokens are high-entropy random strings, so a plain digest
is enough and keeps the handshake cheap.

A dropped stream is not an error condition to recover from — the agent reconnects
with exponential backoff up to 30 seconds, and keeps Xray running the whole time.
On reconnect the panel immediately pushes the current config, so a node converges
without waiting for the next edit.

## Rendering a node's configuration

`hub.BuildNodeConfig` does the following for one node:

1. Load the node's profile document.
2. Keep only the inbounds the node serves.
3. Ask the database for every active user reachable through a squad that grants
   one of those inbounds, as `(user, inbound tag)` pairs.
4. Inject those users as clients into each inbound, in the shape the protocol
   expects — `id` for VLESS/VMess, `password` for trojan and shadowsocks. Each
   client's `email` is `<user-uuid>.<username>`, which is how usage reports are
   attributed back to a user even if a username is later reused.
5. Prepend an internal `dokodemo-door` inbound bound to `127.0.0.1:10085`, enable
   `stats`, add the policy that records per-user counters, and add a routing rule
   that keeps that inbound internal.
6. Serialise deterministically and hash the result with SHA-256.

The hash is the whole synchronisation protocol. The panel skips a push when the
hash matches what the node reported; the agent skips a restart for the same
reason. Identical inputs must always produce an identical hash — there is a test
for exactly that, because a non-deterministic render would restart every node on
every sync.

## Synchronisation

Writes do not push to nodes directly. They call `RequestSync`, which adds the
node to a pending set; a loop drains that set every two seconds. A bulk edit that
touches five hundred users therefore produces one push per node, not five hundred.

The panel narrows the blast radius where it can:

- editing a **profile** syncs the nodes bound to it,
- editing a **user** syncs the nodes whose profiles back that user's squads,
- editing a **squad** syncs everything, since squad membership can span profiles.

## Traffic accounting

The agent reads Xray's stats API with `xray api statsquery -reset`, so every
report is a delta since the previous one and the agent stays stateless. The panel
multiplies each figure by the node's consumption multiplier, then:

- adds it to the user's current and lifetime counters,
- writes an hourly bucket to `user_usage` and `node_usage` for the charts,
- flips a user to `LIMITED` when they cross their quota, and immediately
  re-syncs so they are dropped from the running configuration.

A background loop expires past-due users, rolls traffic counters over according
to each user's reset strategy, and prunes history past `USAGE_RETENTION`.
Heartbeat samples, delivery receipts and subscription request rows each
accumulate at a fixed rate and are pruned on their own schedules, so the tables
behind the charts and the inspectors do not grow without bound.

**Sessions** are derived from the same `user_usage` buckets rather than from a
separate feed. This is a deliberate limit: the panel knows how much a user moved
through which node in a window, and nothing about individual connections or the
addresses they reached, because the agent never reports them. The view says so
rather than implying a precision it does not have.

## Answering a subscription request

`/sub/{token}` and `/s/{token}` resolve the token to a user, then decide what to
serve in a fixed order: an explicit `?format=`, the `User-Agent` when the client
names itself, the first matching response rule, the panel default, and finally
base64. The resolution runs once per request and is reused for the access log,
so a rule's hit counter reflects requests rather than internal lookups.

Clash and sing-box are rendered from a template — the built-in one, or an
operator's own with `{{PROXIES}}`, `{{NAMES}}`, `{{OUTBOUNDS}}`, `{{TAGS}}` and
`{{TITLE}}` splice points. Every request is recorded, including the ones that
resolved to nobody, which is where a revoked link still being polled shows up.

## Notifications

Events are published to an in-process dispatcher with a buffered queue, so a
slow or unreachable endpoint never blocks the request that produced the event.
Webhook bodies are signed with HMAC-SHA256 over `timestamp + "." + body`. A
`4xx` is treated as permanent and not retried; `5xx` and `429` are retried with
exponential backoff. Every attempt is stored with what the endpoint answered, so
"the webhook never arrived" is an answerable question.

Channel secrets — the webhook signing secret and the Telegram bot token — are
write-only: the API never returns them, and a blank field on edit keeps the
stored value.

## Backup

An export reads the configuration tables in one repeatable-read transaction, so
the snapshot is internally consistent; traffic history is excluded because it is
large and reconstructible. Import replaces rather than merges, in a single
transaction, and refuses a snapshot whose schema version does not match rather
than applying it with columns silently dropped. Column types are recorded
alongside the values, because `["vless-reality"]` is ambiguous between a
`text[]` and a `jsonb` without them.

A snapshot carries every credential in the deployment, so both halves are
owner-only and the UI says so.

## Failure handling

- **Bad configuration.** The agent writes the new document, runs
  `xray run -test` against it, and only then restarts. If either step fails it
  restores the previous config, restarts on that, and reports the error — the
  node degrades rather than going dark.
- **Panel restart.** No agent stream survives it, so the panel marks every node
  offline on boot and waits for the agents to reconnect.
- **Node over quota, or disabled.** The panel sends a stop command instead of a
  configuration, and marks the node `TRAFFIC_LIMITED` or `DISABLED`.
- **Slow or wedged agent.** Sends are non-blocking. A message dropped because a
  send buffer is full is logged; the next sync brings the node back in line.
