# 0001. Ship8t v1 scope and architecture

- **Status** Accepted
- **Date** 2026-10-26
- **Deciders** Ryan Jay Leyva

## Context and problem

Ship8t is an independent, open-source static deployment tool. A developer builds
a site, ships a directory with one command, and rolls back when something
breaks. The product plan (draft 1.0) describes a staged vision that ends in
optional managed hosting. It leaves open what the first release is, who it
serves, which commands exist, how it is configured, which platforms it supports,
and how it is tested and released.

ADRs [0002](0002-deployment-states-and-activation.md),
[0003](0003-storage-and-atomic-publish.md), and
[0004](0004-api-and-upload-protocol.md) define the deployment state model,
storage, and wire protocol. This ADR defines everything else about v1 and ties
the set together. When the product plan and an ADR disagree, the ADR wins.

## Requirements and constraints

- Spend nothing on required infrastructure. Everything runs on a developer
  machine or on a small server the user owns.
- Stay independent. Never depend on another deployment service's APIs,
  infrastructure, credentials, or domains.
- Write the client and the server in Go. Never require Node.js at runtime.
- Keep dependencies few and justify each one in an ADR.
- Let a contributor clone the repository and run the full test suite offline
  with `go test ./...`.
- Test security-sensitive behavior from the first commit.
- Do not assume Ship8t operates infrastructure for anyone else. v1 is not a
  hosted service.

## Alternatives considered

**Product shape**

- **Local-only tool.** Smallest scope, but the audience is too small and there
  is nothing to adopt. Rejected.
- **Client that deploys to other hosts.** Delivers value on day one, but it
  hands the immutability and atomic-publish guarantees to each host and abandons
  independence. Rejected.
- **Self-hostable server plus client.** Gives real adopters while Ship8t
  operates nothing. Chosen.

**Packaging**

- **One binary with a `serve` subcommand.** One artifact and one version, but
  every client carries server code and the SQLite dependency, and the two
  programs have different trust profiles. Rejected.
- **Two binaries.** A smaller client, a server installed only where needed, and
  a clear line between the program that holds credentials and local files and
  the program that serves untrusted content. The cost is two artifacts released
  at the same version. Chosen.

**Configuration format**

- **TOML or YAML.** Friendlier to edit and allow comments, but each needs a
  third-party dependency. Rejected.
- **Strict JSON.** Standard library only. Unknown fields fail loudly, so typos
  never pass silently. It has no comments. Chosen.

**Command-line framework**

- **A third-party framework such as cobra.** Provides completion and help
  generation, but v1 has about a dozen commands and that size does not justify
  the dependency. Rejected.
- **Standard library `flag` with hand-written subcommand dispatch.** Chosen.
  Shell completion is deferred.

**Platforms**

- **Add a Windows client.** Adds path-handling and CI burden for an unproven
  audience. Rejected.
- **macOS and Linux on amd64 and arm64.** Chosen.

**Release tooling**

- **GoReleaser.** Convenient, but it adds tooling to learn and trust. Rejected
  for v1.
- **`make dist` with `go build` cross-compilation and checksums.** Chosen.

## Decision

### 1. Product and positioning

Build Ship8t v1 as a self-hostable static deployment platform. Ship two
standalone Go binaries, `ship8t` (client) and `ship8td` (server).

The tagline is "Build it. Ship8t. Roll it back." The one-line description is
"Self-hosted static site deploys with immutable releases, atomic publishing, and
instant rollback."

### 2. Identity

| Item | Value |
|---|---|
| Product name | Ship8t |
| Client binary | `ship8t` |
| Server binary | `ship8td` |
| Go module path | `github.com/ship8t/ship8t` |
| Documentation domain, reserved for the documentation stage | `ship8t.dev` |
| License | Apache License 2.0, with the copyright line "Copyright 2026 Ryan Jay Leyva" in `LICENSE` |
| Versioning | Semantic Versioning, starting at `0.x` until the first stable release |

Create the GitHub organization `ship8t` before the first push. If it is
unavailable, supersede this ADR with a new module path before tagging any
release.

### 3. What v1 includes

- Scan a directory with explicit ignore rules, validate paths, and build a
  checksummed manifest.
- Deploy to a Ship8t server over the protocol in ADR 0004.
- Keep immutable releases, activate them atomically, record history, and roll
  back (ADR 0002 and ADR 0003).
- Serve the active release over HTTP with `<project>.localhost` routing
  (ADR 0004).
- Isolate projects, authenticate with project-scoped tokens, and bound resource
  use.
- Recover from restarts and clean up by retention (ADR 0003).
- Document a local workflow that needs no paid service and no hosted account.

### 4. What v1 excludes

Exclude public production hosting, multi-tenant operation, TLS termination by
the server, public subdomains and custom domains, a web dashboard, user
accounts, billing, a CDN, multiple deployment destinations, build orchestration,
Windows, shell completion, cross-release file reuse, custom response headers and
redirect rules, and any server-side execution of deployed content. Each
exclusion is a deferral, not a ban. Add one later through its own ADR.

### 5. Commands

Implement and test a command before documenting it as working.

| Command | Purpose |
|---|---|
| `ship8t deploy <dir>` | Scan, upload, finalize, and publish. Flags are `--project`, `--server`, `--no-publish`, `--if-active <id>`, `--allow-sensitive-files`, `--insecure-http`, and `--json`. |
| `ship8t deployments list` | List the deployments of a project, newest first. |
| `ship8t deployments inspect <id>` | Show one deployment with its state, manifest summary, and activation history. |
| `ship8t rollback` | Activate an earlier release. Flags are `--project`, `--to <id>`, `--if-active <id>`, and `--json`. |
| `ship8t config show` | Print the effective configuration and the source of each value. Mask tokens. |
| `ship8t login` | Save the server URL and token to the user config. |
| `ship8t logout` | Remove the token from the user config. |
| `ship8t version` | Print the client version and, when a server is configured, the server and API versions. |
| `ship8td serve` | Run the server. Flags are `--config`, `--data-dir`, `--api-listen`, `--sites-listen`, `--base-domain`, and `--dev`. |
| `ship8td token create` | Create a token for `--projects a,b` or `--all-projects`. Print the secret once and store only its hash. |
| `ship8td token list` | List token IDs, scopes, and creation times. Never print secrets. |
| `ship8td token revoke <id>` | Revoke a token. |

Every command accepts `--help`. A usage error prints the relevant usage and
exits with code 2.

`ship8t login --server <url>` reads the token from standard input, or prompts
without echo on a terminal. It never accepts the token as a flag, so the secret
stays out of shell history and process listings. It writes the user config
atomically with mode `0600`. CI pipelines set `SHIP8T_SERVER` and `SHIP8T_TOKEN`
instead of running `login`.

### 6. Configuration

Write every config file as JSON and parse it strictly. Reject unknown fields.

| File | Purpose | Contents |
|---|---|---|
| `ship8t.json` in the project directory | Per-project settings, safe to commit | `project`, `directory`, `ignore`, `include`, `serve.spa_fallback` |
| `$XDG_CONFIG_HOME/ship8t/config.json`, default `~/.config/ship8t/config.json` | Per-user client settings | `server` and `token` |
| `ship8td.json` | Server settings | `data_dir`, `api_listen`, `sites_listen`, `base_domain`, `limits`, `retention`, `idle_timeout` |

Resolve each setting in this order, highest priority first. Command-line flags,
then the environment variables `SHIP8T_SERVER`, `SHIP8T_TOKEN`, and
`SHIP8T_PROJECT`, then `ship8t.json`, then the user config, then built-in
defaults. Reject a `token` key in `ship8t.json`, because that file belongs in
version control.

Refuse to read the user config when its mode allows group or other access. Print
the exact `chmod 600` command in the error.

Require `--data-dir` (or `data_dir` in `ship8td.json`) for `ship8td serve`. Let
`ship8td serve --dev` fall back to `$XDG_DATA_HOME/ship8t`, which defaults to
`~/.local/share/ship8t` on Linux and macOS alike.

| Server setting | Default |
|---|---|
| Site listener | `127.0.0.1:8480` |
| Control API listener | `127.0.0.1:8481` |
| Base domain | `localhost` |
| Limits, retention, idle timeout | As defined in ADR 0003 and ADR 0004 |

When a listener cannot bind, print the address, the cause, and the flag that
changes it.

### 7. Scanning and ignore rules

- Never follow symlinks. Fail on any symlink inside the deploy directory and
  name it.
- Never include anything outside the chosen directory.
- Ignore by default `.git/`, `node_modules/`, `.DS_Store`, `ship8t.json`, and
  every file or directory whose name starts with a dot, except `.well-known/`.
- Apply extra rules from `ignore` in `ship8t.json`. Use `include` to re-add
  paths that a default rule removed.
- Support a small pattern syntax. `*` matches within one path segment, `**`
  matches across segments, `?` matches one character, and a trailing `/` matches
  directories only. Negation with `!` is not supported in v1. There is no
  separate ignore file.
- Fail `deploy` when an included file looks like a secret. Detect by name
  (`.env*`, `id_rsa`, `id_ed25519`, `id_ecdsa`, `*.p12`, `*.pfx`) and by content
  (a `-----BEGIN` line that names a `PRIVATE KEY` within the first 8 KiB of any
  file). Public keys do not match. The error names each file and shows the
  `ignore` rule that excludes it. `--allow-sensitive-files` overrides the check.

### 8. Output, exit codes, and local versus remote

Write human-readable results to standard output and progress and diagnostics to
standard error. Use color only on a terminal and disable it when `NO_COLOR` is
set.

With `--json`, write exactly one JSON document to standard output for `deploy`,
`deployments list`, `deployments inspect`, `rollback`, and `config show`, and
nothing else.

| Exit code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unexpected or unclassified failure |
| 2 | Usage error |
| 3 | Local validation failure (scan, ignore, manifest, limits) |
| 4 | Authentication or authorization failure |
| 5 | Conflict (invalid state, `--if-active` mismatch, incomplete deployment) |
| 6 | Network failure or server unavailable after retries |

State the target server in every `deploy` and `rollback` result, and label it
"local" for a loopback address and "remote" for anything else.

Refuse a non-loopback `http://` server URL unless `--insecure-http` is set, so a
token never travels in clear text by accident. Check the address the client
actually dials, not only the URL text, so a hostname that resolves to loopback
passes. Always allow `https://`.

### 9. Code layout and dependencies

```
cmd/ship8t/        client entry point
cmd/ship8td/       server entry point
internal/          every other package, with no public Go API in v1
docs/adr/          architecture decisions
```

- Keep all packages under `internal/`, so v1 promises no Go library API.
- Build the server as a modular monolith. Split packages only at real
  boundaries, which are manifest validation (shared by client and server),
  scanning, client transport, storage, the deployment state machine, and site
  serving.
- Require Go 1.24 or newer, because the server relies on `os.Root`.
- Allow two direct external dependencies in v1. They are `modernc.org/sqlite`
  (server only, ADR 0003) and `golang.org/x/text/unicode/norm` (NFC
  normalization of manifest paths, which the standard library does not provide).
  Both are BSD-3-Clause licensed and were actively maintained when this ADR was
  written.
- Account for transitive modules. The SQLite driver pulls in several
  `modernc.org/*` modules. Review every `go.sum` change in code review and run
  `govulncheck` in CI.
- Require a recorded justification for any further dependency, covering purpose,
  license, maintenance status, and alternatives. Re-verify the license and
  maintenance status of each dependency when adding it.

### 10. Security boundaries

- Trust only the directory the user names on the client. Trust nothing from the
  client on the server. Treat paths, sizes, checksums, and headers as untrusted
  input.
- Treat deployed content as untrusted. Never execute it, and serve it from a
  different listener and origin than the control API.
- Require a bearer token on every control API route except `GET /v1/info`. Scope
  tokens to projects, store only their hashes, and keep secrets out of logs and
  committed config.
- Let a project reach only its own staging area, releases, and activation
  history. Return `403` for cross-project access and reveal nothing about the
  other project.
- Keep public exposure outside v1. Bind to loopback by default and require an
  explicit flag for any other address. Expect TLS from a reverse proxy. Put rate
  limits, quotas, monitoring, and backup procedures in place before operating
  any public endpoint.

### 11. Testing

- Make `go test ./...` run unit, integration, security, and end-to-end tests
  with no network, credentials, or external service.
- Run the end-to-end test against an in-process server on random loopback ports
  with a temporary data directory, driven through the real client code. Deploy a
  directory, fetch it over HTTP, deploy a newer version, confirm it is active,
  roll back, confirm the earlier content is active again, then simulate a failed
  deployment and confirm the active release is unchanged.
- Cover these security cases. Path traversal, symlinks, malformed manifests,
  oversized manifests and blobs, excessive file counts, checksum mismatch,
  cross-project access, unauthorized requests, invalid state transitions,
  attempts to prune the active release, concurrent publishes, secret detection
  in the scanner, and token redaction in logs.
- Test restart recovery by injecting a failure at each interruption point in the
  recovery table of ADR 0003.
- Run `gofmt` checks, `go vet`, `go test -race ./...`, and `govulncheck ./...`
  in CI. The local run is the authority. Any hosted CI service must be free for
  public repositories.

### 12. Releases and installation

- Build with `make dist`. It cross-compiles `ship8t` and `ship8td` for
  `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64` with
  `CGO_ENABLED=0`, `-trimpath`, and a version stamped through `-ldflags`, then
  writes a `SHA256SUMS` file.
- Publish the binaries and checksums as release assets with installation
  instructions.
- Document `go install github.com/ship8t/ship8t/cmd/ship8t@latest` and the same
  command for `cmd/ship8td` as a supported install path. Read the version from
  the Go build info when no stamped version exists.
- Add checksum signing and build provenance after the first stable release.
  Consider package managers once real usage shows demand.

### 13. Resolution of the product plan's open questions

| Open question | Resolution |
|---|---|
| Project and CLI name | Ship8t, with binaries `ship8t` and `ship8td` (section 2) |
| License | Apache-2.0 (section 2) |
| Operating systems and architectures | macOS and Linux on amd64 and arm64 (section 12) |
| One deployment destination or several | One, a Ship8t server (section 3) |
| Individual files, archive, or streaming upload | Manifest first, then one streamed upload per distinct file (ADR 0004) |
| File-count and size limits | 10,000 files, 50 MiB per file, 500 MiB per deployment, configurable (ADR 0004) |
| Ordering of concurrent deployments | Last committed activation wins, with an optional `--if-active` guard (ADR 0002) |
| Rollback and interruption guarantees | Atomic activation and the recovery table (ADR 0002 and ADR 0003) |
| Number of retained releases | The active release plus the 10 most recent ready releases (ADR 0003) |
| When remote authentication arrives | In v1, as scoped bearer tokens. Accounts wait for optional managed hosting (section 10) |
| Which free host serves the documentation | Deferred to the documentation stage. Decide then, after checking current free-tier terms |
| What demand justifies managed hosting, and its costs | Deferred to the adoption-validation stage. Not a v1 decision |

## Consequences and trade-offs

- v1 reaches real users without Ship8t operating any infrastructure.
- Two binaries mean two artifacts. Release them together at the same version and
  check API compatibility at runtime through `GET /v1/info`.
- Strict JSON catches typos but allows no comments. Users who want comments will
  ask for another format, which a later ADR can supersede.
- Hand-written command dispatch means help text, usage errors, and flag handling
  need their own tests. Shell completion is unavailable in v1.
- Ignoring dotfiles by default protects against leaking secrets and surprises
  users who need other dotfiles. They use `include`, and the `.well-known/`
  exception covers the common case.
- Detecting secrets by name and content catches renamed keys. It adds a read of
  the first 8 KiB of every included file and can produce a rare false positive,
  which `--allow-sensitive-files` resolves.
- Refusing plain `http://` to non-loopback hosts inconveniences home-network
  setups. `--insecure-http` covers them.
- The module path depends on owning the `ship8t` GitHub organization.
- Excluding Windows and TLS keeps v1 small. Operators who want TLS must run a
  reverse proxy, and the documentation must explain how.

## Implementation implications

- Create the repository skeleton with `go.mod` (module
  `github.com/ship8t/ship8t`, Go 1.24), `cmd/ship8t`, `cmd/ship8td`,
  `internal/`, a `Makefile` with `test`, `vet`, `fmt-check`, `vuln`, and `dist`
  targets, and a root `README.md` that describes only implemented behavior.
- Build in this order. Shared manifest validation, then storage and the state
  machine, then the server API and site serving, then the client commands, then
  the end-to-end test. Write the end-to-end test skeleton first and make more of
  it pass at each stage.
- Document each command only after it is implemented and tested. Never present
  an illustrative command as working.
- Update the product plan's `docs/` layout to match the ADRs actually written.

## Status and history

- 2026-10-26. Accepted, together with ADRs 0002, 0003, and 0004.
- 2026-10-26. Revised before any implementation. Renumbered the ADR set so the
  scope ADR comes first, added `ship8t login` and `ship8t logout`, refined
  secret detection to use names and content, changed the default ports to 8480
  and 8481, required `--data-dir` outside `--dev`, and recorded the transitive
  dependency, install, and build details.
- 2026-10-26. Dropped the `NOTICE` file and moved the copyright line into the
  `LICENSE` appendix.
- 2026-10-26. Clarified section 11 without changing the decision. CI also runs
  `staticcheck` as a mandatory check and CodeQL with the `security-extended`
  suite as an advisory check. Neither adds a module dependency.
- 2026-10-27. Clarified section 9 without changing the decision. The manifest
  code also uses the `golang.org/x/text/cases` package, alongside
  `unicode/norm`, to fold letter case when it compares paths. The package is in
  the same `golang.org/x/text` module, so the count of direct dependencies stays
  at two and the license and maintenance record stay the same. The standard
  library has no full case folding, because `strings.ToLower` misses folds such
  as `ß` and `ss`. Pin `golang.org/x/text` to v0.28.0, which declares
  `go 1.23.0`. Release v0.29.0 and later declare `go 1.24.0` or higher, and
  `go get` then raises the `go 1.24` line in `go.mod` to `go 1.24.0`. CI reads
  its Go version from `go.mod`, so that raise changes the toolchain that CI
  installs. Check the `go` line before any upgrade of this module.
- 2026-10-27. Superseded in part by [ADR 0005](0005-minimum-go-version.md). The
  bullet "Require Go 1.24 or newer" in section 9 no longer holds, because
  ADR 0005 requires Go 1.26 or newer. No other decision of this ADR changed.
- 2026-10-27. ADR 0005 pins `golang.org/x/text` to v0.43.0. The earlier entry
  that names v0.28.0 stays as history.
- 2026-10-27. The entry about the x/text pin says that CI reads its Go version
  from `go.mod`. That no longer holds, because `.github/workflows/ci.yml` sets
  `go-version` to `1.26`. See ADR 0005.
- 2026-10-27. Re-verified `golang.org/x/text` when the manifest code began to
  use it, as section 9 requires. The license is BSD-3-Clause. The module
  published a release each month from June to October 2026, most recently
  v0.43.0 on 2026-10-09.
- 2026-10-29. Clarified the order of the rules in section 7 while building
  `internal/scan`, without changing a decision. Apply the default ignore rules
  and the `ignore` patterns before the symlink check, so the rule to fail on a
  symlink covers every entry that the scan keeps. A symlink under an ignored
  path, such as `node_modules/.bin/tool`, is skipped and never followed. Without
  this order, a project with installed packages would often fail on symlinks
  that it never deploys. Match the default rules by name at any depth, including
  `ship8t.json`, and compare names with exact letter case. Apply `node_modules/`
  to directories only, so a symlink with that name is not ignored and fails the
  scan. Keep the `.well-known/` directory, skip a file with that name, and skip
  dot names inside the directory.
- 2026-10-29. Clarified the pattern syntax and `include` in section 7, without
  changing a decision. Match a pattern against the whole path from the deploy
  directory, so `*.map` matches only a file at the root and `**/*.map` matches
  at any depth. Let `**` stand for a whole segment that matches zero or more
  segments, and let `?` match one Unicode character. Reject a pattern that
  starts with `!` or `/`, has an empty segment, or uses `**` inside a segment,
  so a mistake never ignores nothing in silence. Let `include` re-add only paths
  that a default rule removed, and let an `ignore` pattern win over `include`.
  Treat an `include` match on a directory as a match for everything below it,
  including dot names, and open a directory that a default rule skipped when an
  `include` pattern could still match below it.
- 2026-10-29. Clarified secret detection in section 7, without changing a
  decision. Check only the files that the scan keeps, after the ignore rules.
  Match the names on the base name at any depth and ignore letter case, so
  `ID_RSA` matches, and match `.env*` as any base name that starts with `.env`.
  For content, require the `-----BEGIN` marker and `PRIVATE KEY` in capitals on
  one line within the first 8 KiB. Do not require the marker to start the line,
  so a key inside a JSON string matches. A marker that the 8 KiB window cuts is
  not seen. Report every file in one error with one line for each file. Quote
  each path, so a file name cannot forge a line, and show the file path as the
  `ignore` rule to add. The scan code exposes the override as an option, and the
  `--allow-sensitive-files` flag stays with the `deploy` command.
- 2026-10-29. Clarified `ship8t.json` in section 6 while building
  `internal/config`, without changing a decision. Require `directory` to be a
  clean relative path inside the project directory, because the file is
  committed and can come from a repository that the user did not write, so a
  value such as `/` would let `deploy` read and upload any directory. Reject an
  absolute path, a `..` segment, a backslash, a control character, invalid
  UTF-8, and a value over 1024 bytes, and accept `.` for the project directory.
  Require `project` to be the slug of ADR 0003. Read at most 64 KiB, and reject
  a repeated field, a field name with the wrong case, a `null` value, and data
  after the object, so a typo or a repeat never changes a setting in silence.
  Reject a `token` key in any letter case and name the `SHIP8T_TOKEN` variable
  in the error. Treat a missing `ship8t.json` as no settings, and refuse a
  symlink or any other file that is not a regular file, so the file is never
  read from somewhere else.
- 2026-10-29. Clarified the user config in section 6 while building
  `internal/config`, without changing a decision. Require `server` to be an
  `http` or `https` address made of a scheme, a host, and an optional port, and
  reject credentials, a path other than `/`, a query, a fragment, a port outside
  1 to 65535, and any character that is not printable, so an invisible or
  direction changing character cannot make one host look like another when the
  client prints the server. Require `token` to be 1 to 512 visible ASCII
  characters, because it goes into an HTTP header. Read at most 16 KiB, and
  apply the same strict rules as for `ship8t.json`. Never repeat the server or
  the token in an error, because either can hold a secret. Leave the rule on a
  non-loopback `http://` address to the client transport, because it applies to
  the address that the client dials.
- 2026-10-30. Clarified the user config in section 6 while building
  `internal/config`, without changing a decision. Load `config.json` from the
  `ship8t` directory, so the file name is fixed and the caller resolves the
  directory. Treat a missing directory or file as no settings, because a new
  machine has neither. Treat an empty path as an error, because `os.OpenRoot`
  reports it as a missing file and that would hide a caller that forgot to
  resolve the directory. Check the type with `Lstat` before the open, because
  opening a named pipe with no writer blocks, and check type and mode again on
  the opened file before the first read. Refuse a mode with any bit of `0o077`
  set before reading, so a file with a token that others can access is never
  read. Put the `chmod 600` command in the error on one line, with the path in
  single quotes and each quote written as `'\''`, which every common shell reads
  the same way. Omit the command and show the quoted path when the path has a
  control or unprintable character, so a path cannot forge a line or hide a
  command. Follow a symlinked `ship8t` directory, because dotfile managers link
  directories. Refuse a symlinked `config.json`, so the token is never read from
  another file. The message for it says that the refusal is on purpose and
  suggests a link to the directory.
- 2026-10-30. Clarified the command in the user config error of section 6,
  without changing a decision. Write the suggested command as `chmod -- 600`
  followed by the quoted path, so a relative path that starts with a dash is not
  read as an option. Put the `--` before the mode and not after it, because BSD
  `chmod` stops reading options at the mode and would take a later `--` for a
  file name. The command is still the `chmod 600` that section 6 asks for.
- 2026-10-31. Clarified how the client finds the user config and reads the
  environment in section 6 while building `internal/config`, without changing a
  decision. Find the `ship8t` directory in `$XDG_CONFIG_HOME` when that is an
  absolute path, and in `~/.config` otherwise. Ignore a relative
  `XDG_CONFIG_HOME`, because the XDG specification calls it invalid. Fail when
  no absolute base exists, and name `XDG_CONFIG_HOME` and `HOME` in the error,
  because a file that holds a token must never be read from a directory that
  the user did not choose. Do not use `os.UserConfigDir`, because it returns
  `~/Library/Application Support` on macOS and section 6 sets the default to
  `~/.config/ship8t`. Join the path as text and do not clean it, so it names the
  directory that the system opens. Check `SHIP8T_SERVER`, `SHIP8T_TOKEN`, and
  `SHIP8T_PROJECT` with the same rules as the config files. Treat an empty
  variable as unset, so it falls through to the next source, and do not trim a
  value, so a stray space is rejected and not hidden. Name the variable in each
  error and never repeat the server or the token. The order of the sources in
  section 6 is unchanged, and its application is a later step.
- 2026-10-31. Clarified the order of the sources in section 6 while building
  `internal/config`, without changing a decision. Apply the order to each
  setting and use only the sources that can hold it. The server comes from the
  `--server` flag, the environment, and the user config. The token comes from
  the environment and the user config, because no flag carries it and
  `ship8t.json` rejects it. The project comes from the `--project` flag, the
  environment, and `ship8t.json`, because the user config does not hold it.
  Count an empty flag or an empty variable as not given, so it falls through to
  the next source. Validate the flag values with the same rules as the files and
  name the flag in the error, because the flag wins and reaches the dialer.
  Leave a setting empty when no source gives it and do not fail, because section
  6 names no built-in default for these three and each command reports the
  settings it needs. Keep the source of each value for `ship8t config show` in a
  later step.
