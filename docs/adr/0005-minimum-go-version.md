# 0005. Minimum Go version

- **Status** Accepted
- **Date** 2026-10-27
- **Deciders** Ryan Jay Leyva

## Context and problem

ADR 0001, section 9, requires Go 1.24 or newer, because the server relies on `os.Root`. The same section requires `govulncheck` in CI, and CI runs it as its own mandatory job.

The manifest validation package uses `golang.org/x/text` v0.28.0 for NFC normalization and case folding. The vulnerability database lists two advisories against that release.

1. GO-2026-5970. A `norm.Iter` can loop forever on input that contains invalid UTF-8 bytes. `govulncheck` reports the project as affected, because `ValidatePath` and `foldKey` call `norm`, and this fails the `vuln` job. The package checks `utf8.ValidString` before it calls `norm`, so invalid bytes do not reach it today. The scanner cannot see that check.
2. GO-2026-6629. A crafted input can panic `x/text/secure/precis`. The package does not import it, so `govulncheck` lists it only as an advisory in a required module and it does not fail the job.

The fixes ship in x/text v0.39.0 for the first advisory and v0.41.0 for both. Both releases declare `go 1.25.0`, and x/text v0.42.0 and later declare `go 1.26.0`. No patched release works under the Go 1.24 minimum. `govulncheck` has no flag to suppress a finding, so the `vuln` job stays red until the dependency is upgraded.

The standard library matters too. Go 1.25.14 is the last Go 1.25 release. When this ADR was written, 13 advisories at module level and 1 at package level were fixed only in Go 1.26.9 and later. They cover packages including `net/http`, `crypto/tls`, `crypto/x509`, `html/template`, `net/textproto`, `mime/multipart`, and `os`. None sits in code the project calls today, but the server will call `net/http`.

## Requirements and constraints

- Keep `make vuln` passing in CI without suppressing a finding.
- Stay on a Go release that still receives security fixes.
- Keep `os.Root` available.
- Add no fork, copy, or unreviewed dependency, because ADR 0001, section 9, governs dependencies.
- Change the minimum Go version only through an ADR.

## Alternatives considered

- **Stay on Go 1.24 and x/text v0.28.0.** Changes nothing, but leaves a known advisory and a failing `vuln` job. The UTF-8 check keeps invalid bytes away from `norm`, but the scanner cannot see it and the vulnerable code stays in the build. Rejected.
- **Use a patched x/text on Go 1.24.** No such release exists, because every patched release needs Go 1.25 or newer. Rejected.
- **Replace `norm` with project code.** Removes the dependency on the vulnerable package. It needs the Unicode normalization tables and a new implementation to review, which is a larger risk than the advisory. Rejected.
- **Fork or patch x/text, or pin a pseudo-version from its main branch.** A fork is a new dependency without a review trail. A pseudo-version from the main branch declares an equal or newer Go version, so it gains nothing. Rejected.
- **Keep a Go 1.25 minimum with x/text v0.41.0, and use Go 1.26 only in CI.** Clears the `vuln` job and the standard library advisories for CI builds. It does not protect binaries built outside CI, such as a `go install` build, which ADR 0001, section 12, documents, or a local release build. The minimum sets the oldest Go line that can build a release, and no Go 1.25 patch release can fix those advisories. Rejected.
- **Raise the minimum to Go 1.25 and pin x/text v0.41.0.** Clears the `vuln` job today and is the smaller jump. Go 1.25 receives no further fixes, so the standard library advisories fixed only in Go 1.26 stay open, and they can fail the `vuln` job once the server calls `net/http`. This option would force a second raise soon. Rejected.
- **Raise the minimum to Go 1.26 and pin x/text v0.43.0.** Clears the `vuln` job today, runs on a supported Go release, and can take later patch releases for standard library fixes. Chosen.

## Decision

- Require Go 1.26 or newer. Set the module line in `go.mod` to `go 1.26.0`.
- Pin `golang.org/x/text` to v0.43.0, the newest release when this ADR was written. It declares `go 1.26.0` and fixes both advisories.
- Keep every other rule of ADR 0001, section 9, including the limit of two direct dependencies.
- Supersede only the bullet "Require Go 1.24 or newer" in ADR 0001, section 9. `os.Root` has been available since Go 1.24, so ADR 0003 stays true.

### CI toolchain

- `setup-go` reads only the `go` line of `go.mod` and ignores a `toolchain` line. With `go 1.26.0` it installs exactly Go 1.26.0. Without `check-latest`, a version such as `1.26` resolves to the newest 1.26 release already cached on the runner. Set `go-version` to `'1.26'` and `check-latest` to `true` for both jobs in `.github/workflows/ci.yml`, so that `setup-go` installs the newest 1.26 patch release, and keep `go 1.26.0` in `go.mod` as the minimum. Keep the value in `ci.yml` in step with the `go` line.
- A trial run of `make fmt-check vet staticcheck test vuln` passed on exactly Go 1.26.0 and on Go 1.26.10, so this choice is hardening and not a fix. On Go 1.26.0 the scan lists standard library advisories that the code does not call, and on Go 1.26.10 the scan is clean.
- CodeQL does not use `setup-go`. The pinned action bundles a CodeQL version that supports Go up to 1.27. The CodeQL job passed on the first pull request run with no new alerts, so it handles the `go 1.26.0` line.

## Consequences and trade-offs

- The `vuln` job can pass again, and the project can take later Go 1.26 patch releases for standard library fixes without another ADR.
- Contributors need Go 1.26 or newer. With the default `GOTOOLCHAIN=auto`, an older `go` command downloads the right toolchain by itself. The build fails only with `GOTOOLCHAIN=local` or without network access.
- Newer Unicode tables in x/text v0.43.0 can change NFC and case folding results. A trial upgrade in a scratch copy passed every test with the race detector, and the implementation re-runs the checks against a real case-insensitive filesystem.
- Later x/text releases can need an even newer Go version. Check the `go` line of every upgrade of this module.
- The project accepts that the minimum Go version moves again when a dependency or standard library fix needs it.

## Implementation implications

- Update `go.mod` and `go.sum`, and confirm `make fmt-check vet staticcheck test vuln` all pass.
- Keep `go-version` in `.github/workflows/ci.yml` in step with the `go` line of `go.mod`.
- State the new minimum in `README.md`, `CONTRIBUTING.md`, and `docs/go-quality.md`.
- Once this ADR is accepted, add history lines to ADR 0001. One points here and names the superseded bullet. One records the new x/text pin, because the earlier line that names v0.28.0 stays as history. The implementation implications of ADR 0001 and ADR 0003 name Go 1.24 as the original minimum, and those statements stay as written.
- Re-run the manifest tests, the case folding comparison with a real filesystem, and the mutation checks.

## Status and history

- 2026-10-27. Proposed.
- 2026-10-27. Accepted. The CI toolchain open item stays proposed until the implementation verifies it.
- 2026-10-27. Resolved the CI toolchain item for the check and vuln jobs and recorded the result in the Decision section. The CodeQL toolchain stays an open item until a run confirms it. The matching implementation bullet now asks to keep `ci.yml` in step with `go.mod`.
- 2026-10-27. Resolved the CodeQL toolchain item. The CodeQL job passed on the first pull request run with no new alerts, so no open item remains.
