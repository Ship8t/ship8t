# Ship8t

Ship8t is a self-hosted static site deployment tool. The goal for v1 is to
deploy a directory with one command, keep every release immutable, publish a
release atomically, and roll back to an earlier release. It is written in Go and
ships as two binaries, `ship8t` (the client) and `ship8td` (the server).

The project is at the start of v1 implementation. The architecture is decided
and documented, but almost none of it is built yet.

## Current state

The code covers only the start of v1. The file
[docs/current-state.md](docs/current-state.md) lists what exists, package by
package.

Deploying, rollback, the server, authentication, and all of storage except
opening the database and locking the data directory are not implemented. No
command reads a configuration file yet. The commands described in the ADRs do
not exist yet.

## Project structure

Each line says where the part stands today. The file
[docs/current-state.md](docs/current-state.md) has the detail.

- `cmd/ship8t` is the client entry point. It accepts `version` and `help` only.
- `cmd/ship8td` is the server entry point. It accepts `version` and `help`
  only.
- `internal/version` reports the build version for both binaries. Done.
- `internal/manifest` validates and decodes deployment manifests for the client
  and the server. Done. Only the scanner uses it.
- `internal/scan` lists the files of a deploy directory, applies the ignore
  rules, detects secrets, and describes the files for a manifest. Done. No
  command calls it yet.
- `internal/config` reads and validates the config files. The decoders and the
  loaders are done, and no command calls them. The server file `ship8td.json`
  does not exist yet.
- `internal/strictjson` decodes JSON objects strictly for the manifest and the
  config files. Done.
- `internal/deployment` defines the deployment states and checks the moves
  between them. The check is done, and nothing stores a state yet.
- `internal/storage` will own the data directory of the server. It opens the
  SQLite database and locks the data directory. The layout of the data
  directory, the tables, activation, and recovery are still to build, and
  skipped tests mark the recovery, concurrency, and pruning cases.
- `internal/e2e` holds the end-to-end test of the deploy and rollback flow. One
  of its seven stages runs.
- `docs` holds the documentation. `docs/adr` has the architecture decision
  records, `docs/current-state.md` has the state of the code, and
  `docs/go-quality.md` has the Go rules.
- `.github/workflows` holds the GitHub Actions workflows. `ci.yml` runs the
  checks on Linux and macOS and `make vuln` on Linux. `codeql.yml` runs CodeQL,
  and `assign.yml` assigns new issues and pull requests to the maintainer.
- `Makefile` holds the build, test, and analysis targets.
- `CONTRIBUTING.md` explains how to contribute. `AGENTS.md` and `CLAUDE.md` hold
  the rules for AI coding agents. `LICENSE` is the Apache License 2.0.
- `go.mod` and `go.sum` hold the Go module and its two direct dependencies.

All packages live under `internal/`, so v1 offers no public Go API.

## Documentation

- [CONTRIBUTING.md](CONTRIBUTING.md) explains how to set up, which checks to
  run, how to run the fuzz tests, and how to make a change.
- [docs/adr](docs/adr/README.md) is the ADR index and explains the status rules.
  Accepted ADRs are binding.
- [ADR 0001](docs/adr/0001-v1-scope-and-architecture.md) covers v1 scope, the
  command set, configuration, scanning rules, code layout, dependencies,
  testing, and releases.
- [ADR 0002](docs/adr/0002-deployment-states-and-activation.md) covers
  deployment states, allowed transitions, and activation.
- [ADR 0003](docs/adr/0003-storage-and-atomic-publish.md) covers storage layout,
  atomic publishing, crash recovery, and retention.
- [ADR 0004](docs/adr/0004-api-and-upload-protocol.md) covers the HTTP API, the
  manifest format, limits, the upload flow, and local site serving.
- [ADR 0005](docs/adr/0005-minimum-go-version.md) covers the minimum Go version
  and the CI toolchain.
- [docs/current-state.md](docs/current-state.md) lists what exists in the code
  today, package by package.
- [docs/go-quality.md](docs/go-quality.md) lists the Go security and reliability
  rules and the automated checks.
- [AGENTS.md](AGENTS.md) holds the rules for AI coding agents working in this
  repository, including the commit message rules. The file `CLAUDE.md` loads it
  together with `docs/go-quality.md` for Claude Code.

## Development

Go 1.26 or newer is required. These targets exist in the `Makefile`.

- `make build` builds both binaries into `bin/`.
- `make fmt-check` fails when files need `gofmt`.
- `make vet` runs `go vet`.
- `make test` runs `go test -race ./...`.
- `make staticcheck` runs a pinned `staticcheck` release and needs network
  access on first use.
- `make vuln` runs `govulncheck` and needs network access.
- `make dist` cross-compiles both binaries for macOS and Linux on amd64 and
  arm64 and writes a `SHA256SUMS` file.
- `make clean` removes `bin/` and `dist/`.

To try what exists today, run `make build`, then `bin/ship8t version` or
`bin/ship8td help`. These are the only commands. Any other command prints the
usage text to standard error and exits with code 2. A build inside a Git
checkout shows the commit as its version, and any other build shows `dev`. The
file [CONTRIBUTING.md](CONTRIBUTING.md) lists the checks to run before a pull
request and how to run the fuzz tests.

## License

Ship8t is licensed under the Apache License 2.0. See [LICENSE](LICENSE).

## TODO

The items below come from the v1 scope in ADR 0001 and the protocol and storage
ADRs. Nothing in them is implemented unless noted.

Implemented.

- [x] Manifest validation package shared by the client and the server. The
  package is `internal/manifest`. The scanner uses it and the server does not
  use it yet.
- [x] Directory scanner with ignore rules, symlink rejection, and secret
  detection. The package is `internal/scan`. No command calls it yet. Reading
  `ignore` and `include` from `ship8t.json` belongs to the configuration item
  below, and the `--allow-sensitive-files` flag belongs to the `deploy` command
  item.

Partially implemented.

- [ ] `ship8t version` prints the client version only. ADR 0001 also requires
  the server and API versions when a server is configured.
- [ ] Exit codes. Only 0 and 2 are used. ADR 0001 defines codes 0 to 6.
- [ ] Per-command `--help`. Only top-level help exists.
- [ ] Release automation. `make dist` exists, but publishing release assets with
  install instructions does not.
- [ ] End-to-end test and the security test cases listed in ADR 0001. The
  end-to-end test in `internal/e2e` runs one of seven stages. The scanner and
  manifest cases have tests, covering traversal, symlinks, malformed and
  oversized manifests, file counts, and secret detection. The state transition
  table has a test for every pair of states. The cases for recovery, concurrent
  activation, and pruning the active release exist in `internal/storage` as
  skipped placeholders. The other server cases do not exist.
- [ ] Configuration files and precedence. `internal/config` decodes
  `ship8t.json` and the user config, loads `ship8t.json` from a directory, and
  loads the user config with a permission check. Internal helpers find the user
  config directory, validate the environment variables, and apply the
  precedence between sources while keeping the source of each value, and no
  command calls them. `ship8td.json` and passing the project settings to the
  scanner do not exist yet.
- [ ] Deployment state machine. `internal/deployment` checks a move between two
  states, and nothing stores a state yet. Persisted states, activation, and
  history do not exist yet.
- [ ] Storage. `internal/storage` opens the SQLite database with the settings of
  ADR 0003 and locks the data directory. The layout of the data directory, the
  tables, and the file handling do not exist yet.

Not implemented.

- [ ] Atomic activation, history, and crash recovery.
- [ ] Retention and cleanup.
- [ ] Server API with project-scoped bearer tokens and resource limits.
- [ ] Local site serving with `<project>.localhost` routing.
- [ ] Client commands `deploy`, `deployments list`, `deployments inspect`,
  `rollback`, `config show`, `login`, and `logout`.
- [ ] Server commands `serve`, `token create`, `token list`, and `token revoke`.
- [ ] Documentation of a local workflow that needs no paid service.
