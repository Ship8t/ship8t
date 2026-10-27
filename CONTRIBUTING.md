# Contributing to Ship8t

Ship8t is at the start of v1 implementation. The [README](README.md) lists what exists and what is still to do. Pick work from that list, or open an issue first to discuss anything else.

## Before you change anything

- Read the [architecture decision records](docs/adr/README.md). Accepted ADRs are binding.
- To change a decision, write a new ADR that supersedes the old one. Never edit an accepted ADR to change its meaning.
- Read the [Go quality and security rules](docs/go-quality.md).

## Set up

Install Go 1.26 or newer. Clone the repository and run the checks below. No other tools or accounts are needed.

## Run the checks

Run these from the repository root before opening a pull request. CI runs the same checks on Linux and macOS.

```
make fmt-check
make vet
make staticcheck
make test
make vuln
```

`make staticcheck` and `make vuln` need network access. `make build` builds both binaries into `bin/`.

CodeQL also scans each pull request. Its findings are advisory and need a review.

## Make a change

- Keep each pull request to one logical change.
- Add or update tests with every behavior change. Tests must run offline.
- Write tests first for security-sensitive code.
- Update the docs in the same pull request. Document a command only after it is implemented and tested.
- Add no dependency without a recorded justification as described in ADR 0001, section 9.
- Never weaken a security check or a resource limit.

## Layout

- `cmd/ship8t` and `cmd/ship8td` hold the thin entry points.
- `internal/` holds all other packages. Ship8t offers no public Go API in v1.
- `docs/adr` holds the architecture decisions.

## License

Ship8t is licensed under the [Apache License 2.0](LICENSE).
