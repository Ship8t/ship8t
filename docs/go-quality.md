# Go quality and security rules

These rules add to `AGENTS.md`. They cover security and reliability patterns that the style rules do not. Accepted ADRs stay binding. ADR 0001 section 10 and ADRs 0003 and 0004 define the trust boundaries these rules protect.

Apply a rule when its trigger appears in the code you write. Skip it when the trigger is absent. Add a defensive check only when it answers a named threat, an unreliable dependency, or a documented contract. Never add one for a state the code cannot reach.

CodeQL IDs name the matching query in the official Go pack at <https://github.com/github/codeql/tree/main/go/ql/src>. A query is a detector, not the rule itself.

## P0 Security

| Trigger | Required pattern | CodeQL query |
|---|---|---|
| Untrusted input forms a file path | Open files through `os.Root` rooted at the release or staging directory. Reject `..`, absolute paths, and symlinks. Never join input onto a base with `filepath.Join` and trust the result. | `go/path-injection`, `go/zipslip`, `go/unsafe-unzip-symlink` |
| Code starts a process | Call `exec.Command` with a fixed program and an argument list. Never build a shell string from input. | `go/command-injection` |
| Code builds a SQL statement | Pass values as placeholders. Never format input into the statement text. | `go/sql-injection` |
| Code makes an outbound request or redirect from input | Validate the target against the documented rule, such as ADR 0001 section 8 for server URLs. Check the address actually dialed, not only the text. | `go/request-forgery`, `go/unvalidated-url-redirection` |
| Code configures TLS or crypto | Keep certificate verification on. Draw tokens from `crypto/rand`. Hash with SHA-256 and compare secrets with `crypto/subtle`. | `go/disabled-certificate-check`, `go/insecure-tls`, `go/weak-cryptographic-algorithm` |
| Code logs or returns an error | Keep tokens and secrets out of logs and messages. Strip newlines from untrusted values before logging them. | `go/clear-text-logging`, `go/log-injection` |
| Input sets a size, count, or length | Cap request bodies with `http.MaxBytesReader` and streams with `io.LimitReader`. Compare against the ADR 0004 limits before allocating. | `go/uncontrolled-allocation-size`, `go/allocation-size-overflow` |
| Code narrows an integer from input | Range-check before converting, for example `int64` to `int`. | `go/incorrect-integer-conversion` |
| Code writes secrets or temp files | Use mode `0600` for files and `0700` for directories that hold secrets. Create temp files with `os.CreateTemp` in a private directory. | none |

Exception. Fixed internal values, such as a constant path under the data directory, need no validation.

## P1 Correctness and reliability

- Check every error. Check `Close` on files that were written, because a failed close can mean lost data. Reading files may defer `Close` without a check. (`go/missing-error-check`)
- Close every HTTP response body. Drain it only when the connection should be reused.
- Give every `http.Server` read, header, and idle timeouts, and every `http.Client` a timeout or a context deadline. Pass `ctx` as the first parameter to anything that blocks.
- Start a goroutine only when it has an owner and a defined exit path, such as a context or a closed channel. Prefer a plain function call when concurrency adds nothing.
- Guard shared mutable state with one mutex or one owning goroutine. Run `go test -race` on every change.
- Avoid `defer` inside long loops. Wrap the body in a function when each iteration owns a resource.
- Test a security rule with the failing input first, as ADR 0001 section 11 lists.

## P2 Maintainability

Follow the Code style and Comments sections of `AGENTS.md`. Treat `staticcheck` findings as the arbiter of idiomatic Go. Do not mark a style preference as a security rule.

## P3 Performance

Optimize only with a benchmark, a profile, or a documented requirement. Never add a cache, a pool, or extra concurrency on a guess.

## Automated checks

Run these from the repository root. Go 1.26 is the module version. `staticcheck` may download a newer toolchain on first use.

| Command | Checks | Status | Limits |
|---|---|---|---|
| `make fmt-check` | `gofmt` formatting | Mandatory | Formatting only |
| `make vet` | `go vet` suspicious constructs | Mandatory | Current platform only |
| `make staticcheck` | Bugs, deprecated APIs, and idiom | Mandatory | Needs network on first run |
| `make test` | Tests under `-race` | Mandatory | Race detector finds only races that run |
| `make vuln` | Known vulnerabilities in dependencies and the standard library | Mandatory in CI | Needs network, and reports only reachable call paths |
| CodeQL `security-extended` in `.github/workflows/codeql.yml` | Data flow from untrusted input to dangerous sinks | Advisory | Runs on GitHub only. Findings land in code scanning for review |
| `go test -coverprofile=coverage.out ./...` | Untested code | Advisory | No threshold. Coverage does not prove correctness |

CodeQL uses `security-extended` because the server handles untrusted paths and bodies. The `security-and-quality` suite adds quality queries that overlap `go vet` and `staticcheck`, so it is not enabled.

Suppress a finding only with a narrow comment that states why the code is safe. Never disable a check globally to get a clean run.
