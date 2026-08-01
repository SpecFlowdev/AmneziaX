# Contributing

Thanks for wanting to help. This page covers what you need to get a change
merged; [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) explains how the pieces fit
together.

## Getting a development environment

You need Go 1.25+, Node 22+, and a Postgres you can write to. `protoc` is only
needed if you change anything under `proto/`.

```bash
git clone https://github.com/SpecFlowdev/AmneziaX
cd AmneziaX
make ui      # compile the SPA into internal/webui/dist
make build   # bin/amneziax-panel and bin/amneziax-node
make test
```

Run the panel against your own database:

```bash
DATABASE_URL='postgres://user:pass@127.0.0.1:5432/amneziax?sslmode=disable' \
JWT_SECRET=dev-secret \
PANEL_ADMIN_PASSWORD=devpassword \
./bin/amneziax-panel
```

While working on the UI, `cd frontend && npm run dev` proxies `/api` and `/sub`
to `127.0.0.1:8080`, so you get hot reload against a real backend.

To exercise a node without a second server, point the agent at a stub `xray`
that answers `version`, `run -test`, `run -config` and `api statsquery`. The
agent only ever talks to the binary through those four commands.

## Before you open a pull request

```bash
gofmt -l cmd internal      # must print nothing
go vet ./...
go test ./... -race
cd frontend && npm run build
```

CI runs exactly these, plus `shellcheck` on the installers. It also fails if
`internal/webui/dist` is stale — the compiled UI is committed so that a plain
`go build` of a fresh clone produces a working binary, so **commit the rebuilt
bundle whenever you touch `frontend/`**.

## What makes a change easy to merge

- **One thing at a time.** A focused diff gets read; a sweeping one gets
  postponed.
- **Say how you verified it.** Not "should work" — what you ran and what you
  saw. A panel behaviour that was only type-checked has not been tested.
- **Match the surrounding code.** Comments explain *why*, not what the line
  already says. Errors are lowercase and say what failed, not `Error: %v`.
- **Migrations are append-only.** Add `NNNN_description.sql`; never edit one
  that has shipped, because it has already run on other people's databases.
- **Both languages.** New UI text needs a key in `frontend/src/i18n/en.ts` and
  `ru.ts`. The Russian file is typed against the English one, so a missing key
  is a compile error.

## Things worth knowing

**The config hash is the sync protocol.** `xray.Render` must be deterministic:
identical inputs have to produce an identical hash, or every node restarts on
every sync. There is a test for this — keep it passing.

**Node tokens and admin passwords are stored hashed.** If you add a credential,
store a digest, and show the secret exactly once.

**Anything reachable from a subscription URL is unauthenticated.** The
subscription uuid is the only credential a subscriber has, so treat those
handlers as public and never leak another user's data through them.

## Reporting security problems

Please do not open a public issue — see [SECURITY.md](SECURITY.md).
