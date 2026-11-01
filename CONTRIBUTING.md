# Contributing to Ship8t

Ship8t is at the start of v1 implementation. The
[current state](docs/current-state.md) lists what exists, and the
[roadmap](README.md#roadmap) lists what is still to do. No item is open to
outside work yet, so open an issue to discuss any work before you start it.

## Before you change anything

- Read the [architecture decision records](docs/adr/README.md). Accepted ADRs
  are binding.
- To change a decision, write a new ADR that supersedes the old one. Never edit
  an accepted ADR to change its meaning.
- Read the [Go quality and security rules](docs/go-quality.md).

## Set up

Install Go 1.26 or newer. Clone the repository and run the checks below. No
other tools or accounts are needed.

## Run the checks

Run these from the repository root before opening a pull request. CI runs the
same checks on Linux and macOS.

```
make fmt-check
make vet
make staticcheck
make test
make vuln
```

`make staticcheck` and `make vuln` need network access. `make build` builds both
binaries into `bin/`.

CodeQL also scans each pull request. Its findings are advisory and need a
review.

## Run the fuzz tests

`make test` and CI run every fuzz target on its seed inputs and on any failing
input saved under `testdata/fuzz`. They never search for new inputs. Run a
target yourself to search.

These targets exist.

- `FuzzDecode` in `internal/manifest`.
- `FuzzPatternMatches` and `FuzzMayMatchBelow` in `internal/scan`.
- `FuzzDecodeProjectFile` and `FuzzDecodeUserFile` in `internal/config`.

Run one target at a time, from the repository root. The `-fuzz` flag takes one
package and a pattern that matches one target, so `./...` fails. Anchor the
name, because `-fuzz=FuzzDecode` matches both config targets and fails there.

```
go test -run='^$' -fuzz='^FuzzDecode$' -fuzztime=30s ./internal/manifest
```

The run stops after the time you set and prints `PASS` when it finds nothing.
Use `-fuzztime=1x` to check that a target starts.

A failing run saves the input under `testdata/fuzz` in the package. Decide
whether the code or the property of the test was wrong. Commit the saved input
together with the fix when the code was wrong. Delete it when the property was
wrong.

## Make a change

- Keep each pull request to one logical change.
- Add or update tests with every behavior change. Tests must run offline.
- Write tests first for security-sensitive code.
- Update the docs in the same pull request. Document a command only after it is
  implemented and tested.
- Add no dependency without a recorded justification as described in ADR 0001,
  section 9.
- Never weaken a security check or a resource limit.

## Layout

- `cmd/ship8t` and `cmd/ship8td` hold the thin entry points.
- `internal/` holds all other packages. Ship8t offers no public Go API in v1.
- `docs/adr` holds the architecture decisions.

## License

Ship8t is licensed under the [Apache License 2.0](LICENSE).
