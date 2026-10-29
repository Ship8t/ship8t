# 0003. Storage and atomic publish

- **Status** Accepted
- **Date** 2026-10-26
- **Deciders** Ryan Jay Leyva

## Context and problem

The server must store release files and metadata, publish a release atomically,
survive crashes, and serve only the active release. The product plan names
SQLite and the local filesystem as the starting direction. It leaves open how
the two share responsibility, how publication becomes atomic, and how the system
recovers from a crash in the middle of an operation.

Ship8t v1 is a self-hostable server, meaning a single binary on a small server
or a local machine. It is not a multi-node or multi-tenant platform.

## Requirements and constraints

- Never let a request observe a partial or mixed release.
- Preserve the previous active release when a deployment or publication fails.
- Keep finalized releases immutable.
- Isolate projects so one project's data is unreachable through another's paths.
- Define a safe recovery for every interruption point.
- Spend nothing on infrastructure. Use no external database or object store.
- Cross-compile without CGO, so contributors and users need no C toolchain.
- Keep dependencies minimal and justified.
- Support Linux and macOS (amd64 and arm64) as server platforms.
- Run one server process per data directory.

## Alternatives considered

- **Symlink swap on the filesystem.** Keep a `current` symlink per project and
  replace it with `rename(2)`. Atomic on POSIX and free of any database, but it
  has no transactional history, so the activation log and idempotency keys need
  a second mechanism. A crash between the swap and the history write leaves the
  two disagreeing. Rejected.
- **Pointer file replaced by temp file and rename.** Atomic, dependency-free,
  and works on more platforms, but it carries the same consistency gap as the
  symlink swap. Rejected.
- **SQLite as the source of truth for metadata and the active pointer.** One
  transaction updates the pointer and appends the activation log. SQLite also
  holds deployment state, manifests, tokens, and idempotency keys. Files stay on
  the filesystem. Chosen.
- **Client/server database such as Postgres.** More than a single-binary
  self-hosted tool needs, and it adds operational burden. Rejected.
- **Object storage for files.** Outside v1 under the plan's non-goals. Keep file
  access behind a small boundary so a later ADR can add it.

For the SQLite driver, `modernc.org/sqlite` is pure Go, so cross-compiles stay
CGO-free, at some cost in speed and binary size. `mattn/go-sqlite3` is faster
but requires CGO. The workload is small writes and single-pointer reads, so
speed does not matter here. Choose `modernc.org/sqlite`.

## Decision

### Division of responsibility

- Treat SQLite as the source of truth for projects, deployments and their
  states, manifests, the active pointer, the activation log, idempotency keys,
  and API tokens (hashes only).
- Keep only file content on the filesystem. Never let files decide what is
  active.
- Configure the database with WAL journal mode, `synchronous=FULL`, foreign keys
  on, a busy timeout, and a single writer connection. Use `BEGIN IMMEDIATE` for
  activation transactions.
- Take a lock file in the data directory so two server processes never share it.
- Require an explicit data directory as defined in
  [ADR 0001](0001-v1-scope-and-architecture.md).

### Layout

```
<data-dir>/
  ship8t.db
  lock
  projects/<project-id>/
    staging/<deployment-id>/files/...     incomplete content, never served
    releases/<deployment-id>/
      files/...                            finalized content
      manifest.json                        copy of the accepted manifest
  quarantine/                              unexplained directories found
                                           during recovery
  trash/                                   removals in progress
```

Keep `staging/` and `releases/` of a project on the same filesystem, so renaming
a directory between them is atomic. Make project IDs DNS-label slugs
(`[a-z0-9-]`, 1 to 63 characters, no leading or trailing hyphen), validate them,
and never build paths from raw user input. Generate deployment IDs in-house as
time-sortable random IDs.

### Writing files

1. Stream each upload into a temporary file inside the deployment's staging
   directory while computing its SHA-256. Bound memory use and enforce the
   declared size while streaming.
2. When size and checksum match the manifest, flush the file and rename it to
   its final staging path. On a mismatch, delete the temporary file and fail the
   request.
3. Write only to paths derived from the validated manifest, opened through
   `os.Root` rooted at the deployment directory.

### Finalizing

1. Confirm every manifest file is present with the right size. Re-verify the
   checksum of any file not verified during this process's lifetime.
2. Flush files and directories. Set files to `0444` and directories to `0555` as
   defense in depth. The real guarantee is that no code path writes into
   `releases/` after finalization.
3. Rename `staging/<id>` to `releases/<id>`, then commit the `ready` transition
   in SQLite.

### Publishing

Run activation as one SQLite transaction. Check that the target is `ready` and
not `pruned`, check `expected_active` when supplied, update the project's active
pointer, and append the activation log row. Everything commits or nothing does.
Activation touches no files, so no filesystem step can fail halfway.

### Serving

Resolve the request's project, read its active deployment, and open files with
`os.Root` rooted at `releases/<active>/files`. Update the in-memory view after a
publish commits, so a request sees the old release or the new one and never a
mix. Let a request that began before a publish finish against the release it
started on. Open file handles stay valid.

### Crash recovery

Reconcile the filesystem with the database at startup.

| Found | Action |
|---|---|
| Deployment in `created` or `uploading` with a directory in `releases/` | The crash fell between the rename and the commit. Re-verify against the manifest and complete the transition to `ready`. |
| `staging/<id>` for a terminal or unknown deployment | Delete it. |
| `staging/<id>` for a live deployment | Keep it so the client can resume. Idle expiry still applies. |
| Directory in `releases/` with no database row | Move it to `quarantine/`. Never serve it and never delete it automatically. |
| `ready` deployment whose directory is missing | Log an error and mark the project degraded. Never guess or repair. Report active-release problems loudly. |
| Leftover entries in `trash/` | Finish removing them. |

A crash during activation leaves either the old pointer or the new one, because
SQLite transactions are atomic.

### Retention and cleanup

- Keep the active release and the 10 most recent `ready` releases per project.
  Make the count configurable.
- Prune by moving the release to `trash/`, marking it `pruned`, then deleting
  it. Never prune the active release, and test the rejected attempt as a
  security case.
- Expire incomplete deployments after one idle hour (configurable) and remove
  their staging directories.

## Consequences and trade-offs

- The pointer, history, and idempotency state stay consistent by construction,
  with no cross-store protocol.
- SQLite is a real dependency, and the pure-Go driver trades some speed for easy
  cross-compilation and a simple contributor setup.
- The server runs as a single process on a single node. Scaling out is
  impossible in v1, by design.
- The recovery rules add code and tests, and each rule maps to a specific
  interruption point.
- Windows is not a v1 server platform. The layout would mostly work, but rename
  semantics and read-only permissions differ and stay untested.
- The database becomes one more thing to back up. Tell operators to back up
  `ship8t.db` together with `projects/`, and to treat restoring only one of them
  as unsupported.

## Implementation implications

- Add `modernc.org/sqlite` as the server's database dependency and record its
  license and maintenance check when adding it.
- Build one storage package that owns every filesystem write and uses `os.Root`
  for all file access. This requires Go 1.24 or newer.
- Test crash injection at each point in the recovery table, concurrent
  publishes, publishing after a failed deployment, pruning with the active
  release among the candidates, quarantine behavior, and a full restart.
- Document backup and restore procedures before exposing anything publicly.
- Note that ADR 0002 (activation) and ADR 0004 (upload and serving) depend on
  this ADR.

## Status and history

- 2026-10-26. Accepted, together with ADRs 0001, 0002, and 0004.
- 2026-10-26. Renumbered from 0002 to 0003 and reworded before any
  implementation. Added API tokens to the database's responsibilities and the
  explicit data directory requirement. No other decision changed.
