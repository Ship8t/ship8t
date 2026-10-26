# 0001. Ship8t v1 scope and architecture

- **Status** Accepted
- **Date** 2026-10-26
- **Deciders** Ryan Jay Leyva

## Context and problem

Ship8t is an independent, open-source static deployment tool. A developer builds a site, ships a directory with one command, and rolls back when something breaks. The product plan (draft 1.0) describes a staged vision that ends in optional managed hosting. It leaves open what the first release is, who it serves, which commands exist, how it is configured, which platforms it supports, and how it is tested and released.

ADRs [0002](0002-deployment-states-and-activation.md), [0003](0003-storage-and-atomic-publish.md), and [0004](0004-api-and-upload-protocol.md) define the deployment state model, storage, and wire protocol. This ADR defines everything else about v1 and ties the set together. When the product plan and an ADR disagree, the ADR wins.

## Requirements and constraints

- Spend nothing on required infrastructure. Everything runs on a developer machine or on a small server the user owns.
- Stay independent. Never depend on another deployment service's APIs, infrastructure, credentials, or domains.
- Write the client and the server in Go. Never require Node.js at runtime.
- Keep dependencies few and justify each one in an ADR.
- Let a contributor clone the repository and run the full test suite offline with `go test ./...`.
- Test security-sensitive behavior from the first commit.
- Do not assume Ship8t operates infrastructure for anyone else. v1 is not a hosted service.

## Alternatives considered

**Product shape**

- **Local-only tool.** Smallest scope, but the audience is too small and there is nothing to adopt. Rejected.
- **Client that deploys to other hosts.** Delivers value on day one, but it hands the immutability and atomic-publish guarantees to each host and abandons independence. Rejected.
- **Self-hostable server plus client.** Gives real adopters while Ship8t operates nothing. Chosen.

**Packaging**

- **One binary with a `serve` subcommand.** One artifact and one version, but every client carries server code and the SQLite dependency, and the two programs have different trust profiles. Rejected.
- **Two binaries.** A smaller client, a server installed only where needed, and a clear line between the program that holds credentials and local files and the program that serves untrusted content. The cost is two artifacts released at the same version. Chosen.

**Configuration format**

- **TOML or YAML.** Friendlier to edit and allow comments, but each needs a third-party dependency. Rejected.
- **Strict JSON.** Standard library only. Unknown fields fail loudly, so typos never pass silently. It has no comments. Chosen.

**Command-line framework**

- **A third-party framework such as cobra.** Provides completion and help generation, but v1 has about a dozen commands and that size does not justify the dependency. Rejected.
- **Standard library `flag` with hand-written subcommand dispatch.** Chosen. Shell completion is deferred.

**Platforms**

- **Add a Windows client.** Adds path-handling and CI burden for an unproven audience. Rejected.
- **macOS and Linux on amd64 and arm64.** Chosen.

**Release tooling**

- **GoReleaser.** Convenient, but it adds tooling to learn and trust. Rejected for v1.
- **`make dist` with `go build` cross-compilation and checksums.** Chosen.

## Decision

### 1. Product and positioning

Build Ship8t v1 as a self-hostable static deployment platform. Ship two standalone Go binaries, `ship8t` (client) and `ship8td` (server).

The tagline is "Build it. Ship8t. Roll it back." The one-line description is "Self-hosted static site deploys with immutable releases, atomic publishing, and instant rollback."

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

Create the GitHub organization `ship8t` before the first push. If it is unavailable, supersede this ADR with a new module path before tagging any release.

### 3. What v1 includes

- Scan a directory with explicit ignore rules, validate paths, and build a checksummed manifest.
- Deploy to a Ship8t server over the protocol in ADR 0004.
- Keep immutable releases, activate them atomically, record history, and roll back (ADR 0002 and ADR 0003).
- Serve the active release over HTTP with `<project>.localhost` routing (ADR 0004).
- Isolate projects, authenticate with project-scoped tokens, and bound resource use.
- Recover from restarts and clean up by retention (ADR 0003).
- Document a local workflow that needs no paid service and no hosted account.

### 4. What v1 excludes

Exclude public production hosting, multi-tenant operation, TLS termination by the server, public subdomains and custom domains, a web dashboard, user accounts, billing, a CDN, multiple deployment destinations, build orchestration, Windows, shell completion, cross-release file reuse, custom response headers and redirect rules, and any server-side execution of deployed content. Each exclusion is a deferral, not a ban. Add one later through its own ADR.

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

Every command accepts `--help`. A usage error prints the relevant usage and exits with code 2.

`ship8t login --server <url>` reads the token from standard input, or prompts without echo on a terminal. It never accepts the token as a flag, so the secret stays out of shell history and process listings. It writes the user config atomically with mode `0600`. CI pipelines set `SHIP8T_SERVER` and `SHIP8T_TOKEN` instead of running `login`.

### 6. Configuration

Write every config file as JSON and parse it strictly. Reject unknown fields.

| File | Purpose | Contents |
|---|---|---|
| `ship8t.json` in the project directory | Per-project settings, safe to commit | `project`, `directory`, `ignore`, `include`, `serve.spa_fallback` |
| `$XDG_CONFIG_HOME/ship8t/config.json`, default `~/.config/ship8t/config.json` | Per-user client settings | `server` and `token` |
| `ship8td.json` | Server settings | `data_dir`, `api_listen`, `sites_listen`, `base_domain`, `limits`, `retention`, `idle_timeout` |

Resolve each setting in this order, highest priority first. Command-line flags, then the environment variables `SHIP8T_SERVER`, `SHIP8T_TOKEN`, and `SHIP8T_PROJECT`, then `ship8t.json`, then the user config, then built-in defaults. Reject a `token` key in `ship8t.json`, because that file belongs in version control.

Refuse to read the user config when its mode allows group or other access. Print the exact `chmod 600` command in the error.

Require `--data-dir` (or `data_dir` in `ship8td.json`) for `ship8td serve`. Let `ship8td serve --dev` fall back to `$XDG_DATA_HOME/ship8t`, which defaults to `~/.local/share/ship8t` on Linux and macOS alike.

| Server setting | Default |
|---|---|
| Site listener | `127.0.0.1:8480` |
| Control API listener | `127.0.0.1:8481` |
| Base domain | `localhost` |
| Limits, retention, idle timeout | As defined in ADR 0003 and ADR 0004 |

When a listener cannot bind, print the address, the cause, and the flag that changes it.

### 7. Scanning and ignore rules

- Never follow symlinks. Fail on any symlink inside the deploy directory and name it.
- Never include anything outside the chosen directory.
- Ignore by default `.git/`, `node_modules/`, `.DS_Store`, `ship8t.json`, and every file or directory whose name starts with a dot, except `.well-known/`.
- Apply extra rules from `ignore` in `ship8t.json`. Use `include` to re-add paths that a default rule removed.
- Support a small pattern syntax. `*` matches within one path segment, `**` matches across segments, `?` matches one character, and a trailing `/` matches directories only. Negation with `!` is not supported in v1. There is no separate ignore file.
- Fail `deploy` when an included file looks like a secret. Detect by name (`.env*`, `id_rsa`, `id_ed25519`, `id_ecdsa`, `*.p12`, `*.pfx`) and by content (a `-----BEGIN` line that names a `PRIVATE KEY` within the first 8 KiB of any file). Public keys do not match. The error names each file and shows the `ignore` rule that excludes it. `--allow-sensitive-files` overrides the check.

### 8. Output, exit codes, and local versus remote

Write human-readable results to standard output and progress and diagnostics to standard error. Use color only on a terminal and disable it when `NO_COLOR` is set.

With `--json`, write exactly one JSON document to standard output for `deploy`, `deployments list`, `deployments inspect`, `rollback`, and `config show`, and nothing else.

| Exit code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unexpected or unclassified failure |
| 2 | Usage error |
| 3 | Local validation failure (scan, ignore, manifest, limits) |
| 4 | Authentication or authorization failure |
| 5 | Conflict (invalid state, `--if-active` mismatch, incomplete deployment) |
| 6 | Network failure or server unavailable after retries |

State the target server in every `deploy` and `rollback` result, and label it "local" for a loopback address and "remote" for anything else.

Refuse a non-loopback `http://` server URL unless `--insecure-http` is set, so a token never travels in clear text by accident. Check the address the client actually dials, not only the URL text, so a hostname that resolves to loopback passes. Always allow `https://`.

### 9. Code layout and dependencies

```
cmd/ship8t/        client entry point
cmd/ship8td/       server entry point
internal/          every other package, with no public Go API in v1
docs/adr/          architecture decisions
```

- Keep all packages under `internal/`, so v1 promises no Go library API.
- Build the server as a modular monolith. Split packages only at real boundaries, which are manifest validation (shared by client and server), scanning, client transport, storage, the deployment state machine, and site serving.
- Require Go 1.24 or newer, because the server relies on `os.Root`.
- Allow two direct external dependencies in v1. They are `modernc.org/sqlite` (server only, ADR 0003) and `golang.org/x/text/unicode/norm` (NFC normalization of manifest paths, which the standard library does not provide). Both are BSD-3-Clause licensed and were actively maintained when this ADR was written.
- Account for transitive modules. The SQLite driver pulls in several `modernc.org/*` modules. Review every `go.sum` change in code review and run `govulncheck` in CI.
- Require a recorded justification for any further dependency, covering purpose, license, maintenance status, and alternatives. Re-verify the license and maintenance status of each dependency when adding it.

### 10. Security boundaries

- Trust only the directory the user names on the client. Trust nothing from the client on the server. Treat paths, sizes, checksums, and headers as untrusted input.
- Treat deployed content as untrusted. Never execute it, and serve it from a different listener and origin than the control API.
- Require a bearer token on every control API route except `GET /v1/info`. Scope tokens to projects, store only their hashes, and keep secrets out of logs and committed config.
- Let a project reach only its own staging area, releases, and activation history. Return `403` for cross-project access and reveal nothing about the other project.
- Keep public exposure outside v1. Bind to loopback by default and require an explicit flag for any other address. Expect TLS from a reverse proxy. Put rate limits, quotas, monitoring, and backup procedures in place before operating any public endpoint.

### 11. Testing

- Make `go test ./...` run unit, integration, security, and end-to-end tests with no network, credentials, or external service.
- Run the end-to-end test against an in-process server on random loopback ports with a temporary data directory, driven through the real client code. Deploy a directory, fetch it over HTTP, deploy a newer version, confirm it is active, roll back, confirm the earlier content is active again, then simulate a failed deployment and confirm the active release is unchanged.
- Cover these security cases. Path traversal, symlinks, malformed manifests, oversized manifests and blobs, excessive file counts, checksum mismatch, cross-project access, unauthorized requests, invalid state transitions, attempts to prune the active release, concurrent publishes, secret detection in the scanner, and token redaction in logs.
- Test restart recovery by injecting a failure at each interruption point in the recovery table of ADR 0003.
- Run `gofmt` checks, `go vet`, `go test -race ./...`, and `govulncheck ./...` in CI. The local run is the authority. Any hosted CI service must be free for public repositories.

### 12. Releases and installation

- Build with `make dist`. It cross-compiles `ship8t` and `ship8td` for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and `linux/arm64` with `CGO_ENABLED=0`, `-trimpath`, and a version stamped through `-ldflags`, then writes a `SHA256SUMS` file.
- Publish the binaries and checksums as release assets with installation instructions.
- Document `go install github.com/ship8t/ship8t/cmd/ship8t@latest` and the same command for `cmd/ship8td` as a supported install path. Read the version from the Go build info when no stamped version exists.
- Add checksum signing and build provenance after the first stable release. Consider package managers once real usage shows demand.

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
- Two binaries mean two artifacts. Release them together at the same version and check API compatibility at runtime through `GET /v1/info`.
- Strict JSON catches typos but allows no comments. Users who want comments will ask for another format, which a later ADR can supersede.
- Hand-written command dispatch means help text, usage errors, and flag handling need their own tests. Shell completion is unavailable in v1.
- Ignoring dotfiles by default protects against leaking secrets and surprises users who need other dotfiles. They use `include`, and the `.well-known/` exception covers the common case.
- Detecting secrets by name and content catches renamed keys. It adds a read of the first 8 KiB of every included file and can produce a rare false positive, which `--allow-sensitive-files` resolves.
- Refusing plain `http://` to non-loopback hosts inconveniences home-network setups. `--insecure-http` covers them.
- The module path depends on owning the `ship8t` GitHub organization.
- Excluding Windows and TLS keeps v1 small. Operators who want TLS must run a reverse proxy, and the documentation must explain how.

## Implementation implications

- Create the repository skeleton with `go.mod` (module `github.com/ship8t/ship8t`, Go 1.24), `cmd/ship8t`, `cmd/ship8td`, `internal/`, a `Makefile` with `test`, `vet`, `fmt-check`, `vuln`, and `dist` targets, and a root `README.md` that describes only implemented behavior.
- Build in this order. Shared manifest validation, then storage and the state machine, then the server API and site serving, then the client commands, then the end-to-end test. Write the end-to-end test skeleton first and make more of it pass at each stage.
- Document each command only after it is implemented and tested. Never present an illustrative command as working.
- Update the product plan's `docs/` layout to match the ADRs actually written.

## Status and history

- 2026-10-26. Accepted, together with ADRs 0002, 0003, and 0004.
- 2026-10-26. Revised before any implementation. Renumbered the ADR set so the scope ADR comes first, added `ship8t login` and `ship8t logout`, refined secret detection to use names and content, changed the default ports to 8480 and 8481, required `--data-dir` outside `--dev`, and recorded the transitive dependency, install, and build details.
- 2026-10-26. Dropped the `NOTICE` file and moved the copyright line into the `LICENSE` appendix.
- 2026-10-26. Clarified section 11 without changing the decision. CI also runs `staticcheck` as a mandatory check and CodeQL with the `security-extended` suite as an advisory check. Neither adds a module dependency.
