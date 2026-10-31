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
  - The patterns and the options come from the caller. No command passes the
    settings of `ship8t.json` to the scanner yet. The end-to-end test does.
- `internal/config` reads and validates the config files. No command uses it
  yet.
  - `DecodeProjectFile` reads a `ship8t.json` with `project`, `directory`,
    `ignore`, `include`, and `serve.spa_fallback`. `LoadProjectFile` reads it
    from a project directory through `os.Root`, treats a missing file as no
    settings, and refuses a symlink or any other file that is not a regular
    file.
  - `project` must be a slug of lowercase letters, digits, and hyphens, 1 to 63
    characters long. `directory` must be a clean relative path inside the
    project directory, so a committed file cannot point a deploy elsewhere.
  - A `token` key in `ship8t.json` is rejected, because the file belongs in
    version control.
  - `DecodeUserFile` reads the per-user config with `server` and `token`. The
    server must be an `http` or `https` address with printable characters only
    and no credentials, path, query, or fragment. The token must be 1 to 512
    visible ASCII characters.
  - Both decoders accept only exact field names, and reject repeated fields,
    null values, and data after the object. The project decoder reads at most
    64 KiB and the user decoder at most 16 KiB. No error repeats the server or
    the token.
  - `LoadUserFile` reads `config.json` from the `ship8t` directory of the user,
    such as `~/.config/ship8t`, through `os.Root`. A missing file or a missing
    directory means no settings, and an empty path is an error. A symlinked
    directory is followed.
  - `LoadUserFile` refuses a symlinked `config.json` and any other file that is
    not a regular file. It refuses a file that group or other can access before
    it reads a byte, and it checks the opened file again. The error holds the
    `chmod 600` command on one line. It omits the command and quotes the path
    when the path has a control or unprintable character.
  - The package finds the `ship8t` directory. It uses `XDG_CONFIG_HOME` when
    that is an absolute path and `~/.config` otherwise, so it ignores a relative
    `XDG_CONFIG_HOME`. It fails and names `XDG_CONFIG_HOME` and `HOME` when it
    has no absolute base, and it never guesses a directory.
  - The package validates `SHIP8T_SERVER`, `SHIP8T_TOKEN`, and `SHIP8T_PROJECT`
    with the same rules as the config files. An empty variable counts as unset
    and no value is trimmed. An error names the variable and never repeats the
    server or the token.
  - The package picks the server, the token, and the project from the sources
    in the order of ADR 0001. A flag comes first, then the environment, then
    `ship8t.json`, then the user config. Each setting uses only the sources
    that can hold it. The server takes the flag, the environment, and the user
    config. The token takes the environment and the user config. The project
    takes the flag, the environment, and `ship8t.json`.
  - An empty flag counts as not given. The package validates the `--server` and
    `--project` values with the same rules as the files, and the error names
    the flag. A setting that no source gives stays empty, because each command
    reports the settings it needs.
  - Each resolved setting keeps the kind of source that gave it, which is the
    flag, the environment, `ship8t.json`, or the user config. A setting that no
    source gives has no source. The token is kept as it is, so a caller must
    mask it before it prints a setting.
  - None of these helpers is exported, and no command calls them.
- `internal/strictjson` decodes JSON objects for the manifest and the config
  decoders. It rejects unknown fields, names with the wrong case, repeated
  names, and data after the object.
- `internal/deployment` defines the seven deployment states of ADR 0002 and
  checks a move between two of them. No command or server code uses it yet.
  - `Transition` allows the ten moves that the ADR lists, such as `created` to
    `uploading` and `ready` to `pruned`. It rejects every other move with
    `ErrStateConflict`, which the server will answer with `409 state_conflict`.
    A terminal state never changes.
  - A move from a known state to itself succeeds and changes nothing, so a
    repeated request is safe. A repeat of an unknown state is rejected, and so
    is a state name with the wrong letter case.
  - It does not check whether a deployment is active. The activation code will
    refuse to prune the active release.
- `internal/e2e` holds the end-to-end test of deploying and rolling back. One of
  its seven stages runs, and the other six skip until the code they need exists.
  - The running stage loads the `ignore` and `include` rules from the
    `ship8t.json` of a test site and scans the site with them. It then
    describes the files and decodes the manifest as the server will.
- `internal/storage` holds tests and no other code yet. Every test skips until
  the storage code exists, so none of them checks anything today.
  - `TestRecoveryAfterInterruption` lists nine cases from ADR 0003. They are a
    crash before the `ready` commit, the staging directories of dead and live
    deployments, an unknown release directory, a missing release directory,
    leftover pruning entries, a crash during activation, and two
    interruptions of a prune.
  - `TestConcurrentActivation` lists five races between activation operations
    of one project. `TestPruneRefusesActiveRelease` lists two attempts to prune
    the active release.
  - Each case names the code it needs and the check to make once that code
    exists.

Deploying, rollback, the server, storage, and authentication are not
implemented. No command reads a configuration file yet. The commands described
in the ADRs do not exist yet.

## Project structure

- `cmd/ship8t` is the client entry point.
- `cmd/ship8td` is the server entry point.
- `internal/version` reports the build version for both binaries.
- `internal/manifest` validates and decodes deployment manifests for the client
  and the server.
- `internal/scan` lists the files of a deploy directory, applies the ignore
  rules, detects secrets, and describes the files for a manifest.
- `internal/config` reads and validates the config files.
- `internal/strictjson` decodes JSON objects strictly for the manifest and the
  config files.
- `internal/deployment` defines the deployment states and checks the moves
  between them.
- `internal/storage` will own the data directory of the server and holds only
  skipped tests for recovery, concurrent activation, and pruning.
- `internal/e2e` holds the end-to-end test of the deploy and rollback flow.
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

Not implemented.

- [ ] Storage, atomic activation, history, and crash recovery.
- [ ] Retention and cleanup.
- [ ] Server API with project-scoped bearer tokens and resource limits.
- [ ] Local site serving with `<project>.localhost` routing.
- [ ] Client commands `deploy`, `deployments list`, `deployments inspect`,
  `rollback`, `config show`, `login`, and `logout`.
- [ ] Server commands `serve`, `token create`, `token list`, and `token revoke`.
- [ ] Documentation of a local workflow that needs no paid service.
