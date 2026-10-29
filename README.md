# Ship8t

Ship8t is a self-hosted static site deployment tool. The goal for v1 is to
deploy a directory with one command, keep every release immutable, publish a
release atomically, and roll back to an earlier release. It is written in Go and
ships as two binaries, `ship8t` (the client) and `ship8td` (the server).

The project is at the start of v1 implementation. The architecture is decided
and documented, but almost none of it is built yet.

## Current state

Only the following exists in the code today.

- `ship8t` and `ship8td` each accept `version`, which prints the build version,
  and `help`, which prints the usage text.
- Any other command prints the usage text to standard error and exits with
  code 2.
- The version comes from a build-time stamp, then from the Go module build info,
  then falls back to `dev`.
- `internal/manifest` validates and decodes deployment manifests. A manifest
  lists every file of a deployment with its path, size, and SHA-256 checksum, as
  ADR 0004 defines.
  - `Decode` reads at most 8 MiB and accepts only a manifest version it
    supports.
  - It accepts only exact field names, rejects repeated names and trailing data,
    and requires a `size` for every file.
  - It rejects unsafe paths. These include traversal, reserved Windows names,
    control characters, invalid UTF-8, and names that are not in Unicode NFC
    form.
  - It rejects names that differ only by letter case or Unicode normalization,
    because they would clash on a case-insensitive filesystem.
  - It enforces limits on the file count and the file sizes. The defaults are
    10,000 files, 50 MiB per file, and 500 MiB in total.
  - `Validate` applies the same rules and limits to a manifest that code builds
    in memory.
  - Only `internal/scan` uses it. No command or server code uses it yet.
- `internal/scan` lists the files of a deploy directory and describes them as
  manifest entries. No command uses it yet.
  - `Scan` reads a directory through `os.Root` and returns the sorted,
    slash-separated paths of its regular files.
  - It fails on any symlink and on any other file that is not a regular file,
    and it names the path. A symlink under an ignored path is skipped instead.
  - It skips `node_modules/`, `ship8t.json`, and every file or directory whose
    name starts with a dot, except `.well-known/`.
  - It applies extra `ignore` patterns and `include` patterns that add skipped
    paths back. The syntax is `*` within one segment, `**` across segments, `?`
    for one character, and a trailing `/` for directories only. Negation with
    `!` is rejected. A pattern matches the whole path from the deploy directory,
    and an `ignore` pattern wins over an `include` pattern.
  - It fails when a listed file looks like a secret. The name rules are `.env*`,
    `id_rsa`, `id_ed25519`, `id_ecdsa`, `*.p12`, and `*.pfx`, and they ignore
    letter case. The content rule is a `-----BEGIN` line that names a
    `PRIVATE KEY` in the first 8 KiB. The error names each file and shows the
    `ignore` rule that excludes it. An option turns the check off.
  - `Describe` returns the size and the SHA-256 checksum of each file. It checks
    the paths and the limits with `manifest.Validate` before it reads any
    content.
  - The patterns and the options come from the caller. Nothing reads
    `ship8t.json` yet.

Deploying, rollback, the server, storage, authentication, and configuration
files are not implemented. The commands described in the ADRs do not exist yet.

## Project structure

- `cmd/ship8t` is the client entry point.
- `cmd/ship8td` is the server entry point.
- `internal/version` reports the build version for both binaries.
- `internal/manifest` validates and decodes deployment manifests for the client
  and the server.
- `internal/scan` lists the files of a deploy directory, applies the ignore
  rules, detects secrets, and describes the files for a manifest.
- `docs/adr` holds the architecture decision records.
- `Makefile` holds the build, test, and analysis targets.
- `.github/workflows` holds the GitHub Actions workflows for CI and CodeQL.

All packages live under `internal/`, so v1 offers no public Go API.

## Documentation

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
- [docs/go-quality.md](docs/go-quality.md) lists the Go security and reliability
  rules and the automated checks.
- [AGENTS.md](AGENTS.md) holds the rules for AI coding agents working in this
  repository, including the commit message rules.

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

Not implemented.

- [ ] Configuration files and precedence (`ship8t.json`, user config,
  `ship8td.json`, environment variables).
- [ ] Storage, the deployment state machine, atomic activation, history, and
  crash recovery.
- [ ] Retention and cleanup.
- [ ] Server API with project-scoped bearer tokens and resource limits.
- [ ] Local site serving with `<project>.localhost` routing.
- [ ] Client commands `deploy`, `deployments list`, `deployments inspect`,
  `rollback`, `config show`, `login`, and `logout`.
- [ ] Server commands `serve`, `token create`, `token list`, and `token revoke`.
- [ ] End-to-end test and the security test cases listed in ADR 0001.
- [ ] Documentation of a local workflow that needs no paid service.
