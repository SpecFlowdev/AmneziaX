## What this changes

<!-- One or two sentences. Link the issue it closes, if there is one. -->

## Why

<!-- The problem being solved, not a restatement of the diff. -->

## How it was verified

<!--
What you actually ran. "Built and it compiles" is not verification; say which
behaviour you exercised and what you saw.
-->

- [ ] `make test` passes
- [ ] `go vet ./...` is clean and `gofmt -l cmd internal` is empty
- [ ] `cd frontend && npm run build` succeeds, and the bundle in
      `internal/webui/dist` is committed if the UI changed
- [ ] Checked against a running panel, not only in tests

## Notes for the reviewer

<!-- Anything surprising, deliberately left out, or worth a second opinion. -->
