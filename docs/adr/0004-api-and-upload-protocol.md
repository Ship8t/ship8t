# 0004. API and upload protocol

- **Status** Accepted
- **Date** 2026-10-26
- **Deciders** Ryan Jay Leyva

## Context and problem

The client and the server need a versioned HTTP protocol to create deployments,
transfer files, finalize releases, publish, roll back, and read history. The
product plan requires the protocol to define request and response formats,
checksum validation, limits, errors, retries, idempotency, authentication, and
compatibility rules, and to avoid loading whole deployments into memory. The
protocol must also stay independent of any other service's API. Locally served
sites need a routing rule as well.

## Requirements and constraints

- Bound memory on both client and server, whatever the deployment size.
- Make every request safe to retry, and let an interrupted upload resume.
- Verify every accepted byte against a client-declared checksum.
- Treat all paths, sizes, and metadata as untrusted and validate them again on
  the server.
- Return structured, machine-readable errors.
- Require authentication on the control API, even for local use.
- Keep deployed content, which is untrusted, from reaching the control API with
  ambient credentials.
- Use only the standard library `net/http`, with no framework.

## Alternatives considered

- **Manifest first, then per-file uploads, then finalize.** Each file retries on
  its own, memory stays bounded, and every step is idempotent. It also leaves
  room to skip files the server already holds, with no protocol change. Chosen.
- **Single archive upload (tar or zip).** The simplest client, but one failure
  restarts the whole transfer. It also invites archive attacks (path traversal
  in entries, decompression bombs, symlink entries) and needs server-side
  extraction. Rejected.
- **Streaming or multipart single request.** Fewer round trips, but weak retry,
  poor resume, and awkward limit enforcement. Rejected.
- **Chunked or resumable upload protocols such as tus.** Too complex for static
  sites of this size. Revisit only if large files matter. Rejected.

For addressing uploads, the product plan sketches paths in URLs. Paths in URLs
suffer from encoding ambiguity, proxy normalization, and `..` handling. Address
each upload by its SHA-256 instead, which keeps untrusted paths out of URLs
entirely. The server writes the blob to every manifest path that declares that
checksum, and storage stays path-based. If blob addressing proves awkward, fall
back to a path in a query parameter.

## Decision

### General rules

- Serve every route under `/v1` with JSON bodies (`application/json`) and no
  cookies.
- Require `Authorization: Bearer <token>` on every control API route except
  `GET /v1/info`. Never accept a token in a query string.
- Run the control API and site serving on separate listeners with separate
  ports, and therefore separate origins. Send no CORS headers from the API, so
  browsers cannot make credentialed cross-origin calls to it from deployed
  pages.
- Bind to `127.0.0.1` by default. Require an explicit flag to bind elsewhere.
  Leave TLS to a reverse proxy in v1.
- Report errors as `application/problem+json` (RFC 9457) with `type`, `title`,
  `status`, `detail`, a stable machine-readable `code`, and per-item `errors`
  where relevant.

### Authentication and authorization

- Create tokens with `ship8td token create` and store only their SHA-256 hashes
  in the server database. Compare in constant time.
- Scope each token to all projects or to an explicit list of project IDs. Return
  `403 forbidden` for requests outside the scope, and reveal nothing about other
  projects.
- Make `ship8td serve --dev` generate a token and write it to a `0600` file in
  the data directory. It uses the same code path as production and is no weaker.
- Redact tokens and `Authorization` headers from every log, and cover that with
  a test.

### Routes

| Method and path | Purpose |
|---|---|
| `GET /v1/info` | Return the API version, server version, and limits. No authentication. |
| `PUT /v1/projects/{project_id}` | Create the project if it does not exist. Idempotent. |
| `POST /v1/projects/{project_id}/deployments` | Create a deployment from a manifest and return what to upload. |
| `PUT /v1/deployments/{deployment_id}/blobs/{sha256}` | Upload one file's content as a streamed body. |
| `POST /v1/deployments/{deployment_id}/finalize` | Verify and finalize the release. |
| `POST /v1/deployments/{deployment_id}/cancel` | Cancel an unfinished deployment. |
| `POST /v1/deployments/{deployment_id}/publish` | Activate a `ready` deployment. Accept an optional `expected_active`. |
| `POST /v1/projects/{project_id}/rollback` | Activate an earlier release. Accept optional `target` and `expected_active`. |
| `GET /v1/projects/{project_id}/deployments` | List deployments, paginated, newest first. |
| `GET /v1/deployments/{deployment_id}` | Return a deployment with its state and manifest. |
| `GET /v1/projects/{project_id}/activations` | Return the activation log. |

### Manifest (version 1)

```json
{
  "manifest_version": 1,
  "files": [
    {
      "path": "index.html",
      "size": 1234,
      "sha256": "<64 lowercase hex characters>"
    }
  ],
  "serve": { "spa_fallback": false }
}
```

The client validates the manifest for early feedback, and the server validates
it again, because the server never trusts the client.

- Reject an unsupported `manifest_version` with
  `400 unsupported_manifest_version`. Reject unknown fields so a typo cannot
  silently change behavior.
- Require at least one file.
- Require relative paths that use `/` only, are valid UTF-8 in NFC form, and
  stay within 1024 bytes in total and 255 bytes per segment.
- Forbid a leading `/`, empty segments, `.` and `..` segments, backslashes, NUL
  and other control characters, Windows reserved names (`CON`, `PRN`, `AUX`,
  `NUL`, `COM1` to `COM9`, `LPT1` to `LPT9`), and segments that end in a dot or
  a space.
- Reject duplicate paths and paths that differ only by case or Unicode
  normalization, so a release behaves identically on case-insensitive
  filesystems.
- Reject a path that is both a file and the directory prefix of another file.
- Never allow symlinks in a manifest. The client scanner rejects them and never
  follows them.
- Require `size` to be a non-negative integer and `sha256` to be 64 lowercase
  hex characters.
- Limit the manifest request body to 8 MiB.

### Limits

Ship these defaults. Make them configurable on the server and report them
through `GET /v1/info`.

| Limit | Default |
|---|---|
| Files per deployment | 10,000 |
| Size of one file | 50 MiB |
| Total size of a deployment | 500 MiB |
| Unfinished deployments per project | 10 |
| Idle time before an unfinished deployment expires | 1 hour |

Return `413` or `422` with code `limit_exceeded` and name the limit that was
exceeded.

### Upload flow

1. Send `POST .../deployments` with the manifest. The server validates it,
   enforces limits, and returns `201` with the deployment `id`, its `state`,
   `expires_at`, and `upload.missing`, the list of `{sha256, size}` entries the
   server still needs. In v1 that list holds every distinct blob.
2. For each missing blob, send `PUT .../blobs/{sha256}` with `Content-Length`
   and the raw bytes. The server streams to a temporary file while hashing and
   enforcing the declared size. It answers `422 checksum_mismatch` on any
   mismatch and removes the temporary file. It answers `204` for a blob already
   stored for this deployment without rewriting it. It rejects any `sha256` the
   manifest does not reference.
3. Send `POST .../finalize`. If anything is missing, the server answers
   `409 incomplete` with the remaining `missing` list and leaves the deployment
   in `uploading`. When everything is present, it finalizes
   ([ADR 0003](0003-storage-and-atomic-publish.md)) and returns the `ready`
   deployment.
4. Send `POST .../publish`. The client does this automatically unless
   `--no-publish` is set. The response contains the activation record and the
   site URL.

Let the client upload with a small bounded number of concurrent requests
(default 4), streaming from disk.

### Retries and idempotency

- `PUT blobs` is idempotent by nature, since the same checksum always carries
  the same bytes. `finalize` and `cancel` are idempotent as defined in
  [ADR 0002](0002-deployment-states-and-activation.md).
- Accept an `Idempotency-Key` header on the `POST` requests that create or
  activate things (create deployment, publish, rollback). Store the key with a
  hash of the request for 24 hours. Return the original response for a repeat
  with the same key and request, and `422 idempotency_key_reused` for the same
  key with a different request.
- Make the client retry network errors, `429`, and `5xx` with exponential
  backoff and jitter, up to 5 attempts, and honor `Retry-After`. Never retry
  other `4xx` responses.
- Find a resumed deployment again by its ID. `GET /v1/deployments/{id}` returns
  the current `missing` list.

### Compatibility

- Treat the `/v1` path as the compatibility boundary. Within `v1`, the server
  may add optional fields, endpoints, and error codes. Make clients ignore
  unknown response fields and treat unknown error codes as a generic failure of
  the matching HTTP status.
- Require `/v2` and a new ADR for any breaking change. The manifest carries its
  own version, so manifest changes never force an API version change.
- Make the client read `GET /v1/info` first and report a clear, actionable
  message when the server is too old or too new, rather than failing in the
  middle of an upload.

### Local site serving

- Serve sites on their own listener at `http://<project-id>.localhost:8480/` by
  default. `*.localhost` resolves to loopback in common browsers, `curl`, and
  Go, so no DNS or TLS setup is needed and root-relative URLs such as
  `/style.css` work. Reject path-prefix routing (`/<project>/`), because it
  breaks those URLs.
- Parse the `Host` header strictly. Return `404` for anything that is not a
  valid project slug under the configured base domain. Public hostnames and TLS
  stay outside this ADR.
- Map `/` and directory paths to `index.html`. Map `/about` to `about.html` or
  `about/index.html`. Return the release's `404.html` for a missing path when it
  exists, and a plain `404` otherwise. When `serve.spa_fallback` is true, answer
  a missing path with no file extension with `index.html` and status `200`.
- Send `Content-Type` from the file extension,
  `X-Content-Type-Options: nosniff`, an `ETag` derived from the manifest
  checksum, and `Cache-Control: no-cache` for HTML. Apply a conservative default
  to other assets. Leave header customization out of v1.
- Reach only the active release of the matching project, and open files through
  `os.Root`, which rejects traversal.
- Never execute deployed files.

## Consequences and trade-offs

- Uploads resume, retry, and stay bounded in memory, at the price of several
  requests per deployment. Concurrent uploads keep that acceptable for typical
  static sites (hundreds to a few thousand files), and the 10,000-file default
  bounds the worst case.
- Addressing blobs by checksum keeps hostile paths out of URLs and deduplicates
  within a deployment for free. It also forces the server to copy or link one
  blob into several paths.
- Skipping unchanged files between releases is not in v1. The `missing` list is
  the extension point, so adding it later changes server behavior and not the
  wire format.
- Separate listeners and mandatory tokens add a little setup friction for local
  use. In return, a malicious deployed page cannot call the control API.
- Strict manifest validation rejects some sites that other tools accept, such as
  files that differ only by case. That is intentional, and the error names the
  offending paths.
- Supporting `*.localhost` depends on the client resolving it. Tools that do not
  resolve it need an explicit `Host` header, and the documentation must say so.

## Implementation implications

- Share one manifest validation package between the client scanner and the
  server. Cover every rule above with table-driven tests, including traversal,
  case collisions, Unicode normalization, reserved names, and symlinks.
- Test the server against oversized manifests and blobs, too many files,
  checksum mismatch, a blob missing from the manifest, finalize with missing
  blobs, repeated and concurrent requests, idempotency key reuse, unauthorized
  and cross-project access, and token redaction in logs.
- Give the client a retry-and-backoff HTTP transport and bounded parallel
  streaming uploads.
- Document the protocol under `docs/api/` once the implementation exists.
  Describe only implemented behavior in user-facing docs.
- Leave these topics to later ADRs. Remote authentication beyond static tokens,
  public hostnames and TLS, custom headers and redirects, and cross-release file
  reuse.

## Status and history

- 2026-10-26. Accepted, together with ADRs 0001, 0002, and 0003.
- 2026-10-26. Renumbered from 0003 to 0004 and reworded before any
  implementation. Changed the server command to `ship8td serve --dev`, moved
  token storage to the database, and set the default site port to 8480. No other
  decision changed.
- 2026-10-27. Clarified the manifest rules while building `internal/manifest`,
  without changing a decision. Require a `size` in every file entry, where `0`
  is an explicit empty file and a missing or null value is an error. Reject
  Windows reserved names with or without an extension, such as `nul.txt`. Match
  field names with exact case and reject a repeated field name, because the Go
  JSON decoder would otherwise match names without regard to case and keep the
  last repeated value. Require `serve` to be a JSON object when present.
- 2026-10-27. Clarified UTF-8 and collision handling. Reject a path that
  contains U+FFFD, because the Go JSON decoder replaces invalid UTF-8 bytes and
  lone surrogate escapes with it before any path check runs. This also rejects a
  real file name that contains that character. Compare paths for collisions
  after NFD normalization and case folding, so names that a case-insensitive and
  normalization-insensitive volume such as APFS treats as one file are rejected
  too. Fold with the `golang.org/x/text/cases` package and also map each rune
  through `unicode.SimpleFold`, because the x/text tables miss some writing
  systems such as Cherokee. Both use the Unicode tables of their library
  version, so a letter added in a newer Unicode version is not folded until
  those tables are updated. Two names that differ only in the case of such a
  letter can then pass validation although a case-insensitive volume treats them
  as one file.
- 2026-10-27. Clarified limit enforcement. Count files while reading the `files`
  array and stop at the limit, so a small body cannot expand into a large
  allocation. Read at most 8 MiB of the manifest body. Cap the text of a decode
  error that quotes untrusted input, so an error never grows with the body.
