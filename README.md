# Ship8t

Ship8t is a self-hosted static site deployment tool. The goal for v1 is to deploy a directory with one command, keep every release immutable, publish a release atomically, and roll back to an earlier release. It is written in Go and ships as two binaries, `ship8t` (the client) and `ship8td` (the server).

The project is at the start of v1 implementation. The architecture is decided and documented, but almost none of it is built yet.

## Current state

Only the following exists in the code today.

- `ship8t` and `ship8td` each accept `version`, which prints the build version, and `help`, which prints the usage text.
- Any other command prints the usage text to standard error and exits with code 2.
- The version comes from a build-time stamp, then from the Go module build info, then falls back to `dev`.
- `internal/manifest` validates and decodes deployment manifests. A manifest lists every file of a deployment with its path, size, and SHA-256 checksum, as ADR 0004 defines.
  - `Decode` reads at most 8 MiB and accepts only a manifest version it supports.
  - It accepts only exact field names, rejects repeated names and trailing data, and requires a `size` for every file.
  - It rejects unsafe paths. These include traversal, reserved Windows names, control characters, invalid UTF-8, and names that are not in Unicode NFC form.
  - It rejects names that differ only by letter case or Unicode normalization, because they would clash on a case-insensitive filesystem.
  - It enforces limits on the file count and the file sizes. The defaults are 10,000 files, 50 MiB per file, and 500 MiB in total.
  - No command or server code uses it yet.

Deploying, rollback, the server, storage, authentication, and configuration files are not implemented. The commands described in the ADRs do not exist yet.

## Project structure

- `cmd/ship8t` is the client entry point.
- `cmd/ship8td` is the server entry point.
- `internal/version` reports the build version for both binaries.
- `internal/manifest` validates and decodes deployment manifests for the client and the server.
- `docs/adr` holds the architecture decision records.
- `Makefile` holds the build, test, and analysis targets.
- `.github/workflows` holds the GitHub Actions workflows for CI and CodeQL.

All packages live under `internal/`, so v1 offers no public Go API.

## Documentation

- [docs/adr](docs/adr/README.md) is the ADR index and explains the status rules. Accepted ADRs are binding.
- [ADR 0001](docs/adr/0001-v1-scope-and-architecture.md) covers v1 scope, the command set, configuration, scanning rules, code layout, dependencies, testing, and releases.
- [ADR 0002](docs/adr/0002-deployment-states-and-activation.md) covers deployment states, allowed transitions, and activation.
- [ADR 0003](docs/adr/0003-storage-and-atomic-publish.md) covers storage layout, atomic publishing, crash recovery, and retention.
- [ADR 0004](docs/adr/0004-api-and-upload-protocol.md) covers the HTTP API, the manifest format, limits, the upload flow, and local site serving.
- [docs/go-quality.md](docs/go-quality.md) lists the Go security and reliability rules and the automated checks.
- [AGENTS.md](AGENTS.md) holds the rules for AI coding agents working in this repository, including the commit message rules.

## Development

Go 1.24 or newer is required. These targets exist in the `Makefile`.

- `make build` builds both binaries into `bin/`.
- `make fmt-check` fails when files need `gofmt`.
- `make vet` runs `go vet`.
- `make test` runs `go test -race ./...`.
- `make staticcheck` runs a pinned `staticcheck` release and needs network access on first use.
- `make vuln` runs `govulncheck` and needs network access.
- `make dist` cross-compiles both binaries for macOS and Linux on amd64 and arm64 and writes a `SHA256SUMS` file.

## License

Ship8t is licensed under the Apache License 2.0. See [LICENSE](LICENSE).

## TODO

The items below come from the v1 scope in ADR 0001 and the protocol and storage ADRs. Nothing in them is implemented unless noted.

Implemented.

- [x] Manifest validation package shared by the client and the server. The package is `internal/manifest`. The scanner and the server do not use it yet, and symlink rejection stays with the scanner item below.

Partially implemented.

- [ ] `ship8t version` prints the client version only. ADR 0001 also requires the server and API versions when a server is configured.
- [ ] Exit codes. Only 0 and 2 are used. ADR 0001 defines codes 0 to 6.
- [ ] Per-command `--help`. Only top-level help exists.
- [ ] Release automation. `make dist` exists, but publishing release assets with install instructions does not.

Not implemented.

- [ ] Directory scanner with ignore rules, symlink rejection, and secret detection.
- [ ] Configuration files and precedence (`ship8t.json`, user config, `ship8td.json`, environment variables).
- [ ] Storage, the deployment state machine, atomic activation, history, and crash recovery.
- [ ] Retention and cleanup.
- [ ] Server API with project-scoped bearer tokens and resource limits.
- [ ] Local site serving with `<project>.localhost` routing.
- [ ] Client commands `deploy`, `deployments list`, `deployments inspect`, `rollback`, `config show`, `login`, and `logout`.
- [ ] Server commands `serve`, `token create`, `token list`, and `token revoke`.
- [ ] End-to-end test and the security test cases listed in ADR 0001.
- [ ] Documentation of a local workflow that needs no paid service.
