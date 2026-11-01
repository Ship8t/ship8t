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
  pruning/<project-id>/<deployment-id>/    releases being pruned
```

Keep `staging/`, `releases/`, and `pruning/` on the same filesystem, so renaming
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
| Leftover entries in `pruning/` | Finish removing them. |

A crash during activation leaves either the old pointer or the new one, because
SQLite transactions are atomic.

### Retention and cleanup

- Keep the active release and the 10 most recent `ready` releases per project.
  Make the count configurable.
- Prune by moving the release to `pruning/<project-id>/<deployment-id>`, marking
  it `pruned`, then deleting it. Never prune the active release, and test the
  rejected attempt as a security case.
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
- 2026-10-31. Renamed `trash/` to `pruning/` and settled crash recovery after
  an interrupted prune, which this ADR left open. No other decision changed.
  The layout, the recovery table, and retention and cleanup use the new name.
  No storage code exists yet, so no data or code used the old name. The new name
  says what the directory holds, as `staging/` does for uploads, and it matches
  the `pruned` state. Name each entry `pruning/<project-id>/<deployment-id>`, so
  the path alone identifies the project and the release, and never restore a
  release from it. Keep `pruning/` on the same filesystem as `releases/`, so
  moving a release into it is an atomic rename. Prune moves a release to
  `pruning/`, marks it `pruned`, then deletes it, so a crash after the move and
  before the mark leaves a `ready` deployment whose directory sits in
  `pruning/`. Do not read that directory as missing. Check each entry of
  `pruning/` against the database before the row for a missing directory and
  before the row for leftover entries. When the entry belongs to a `ready`
  deployment that is not the active release, finish the prune. Mark the
  deployment `pruned` in one transaction and then delete the entry, so a crash
  between the two steps leaves the leftover that the `pruning/` row already
  finishes. Retention chose that release, so finishing is the outcome that the
  crash interrupted, and restoring the directory would undo a decision that was
  already made. Never finish a prune for the active release, because retention
  never prunes it, and do not apply the row for leftover entries to it either.
  Treat an active release whose directory sits in `pruning/` as a missing
  directory, which means log an error, mark the project degraded, and repair
  nothing.
- 2026-10-31. Clarified how the database opens while building
  `internal/storage`, without changing a decision. Set the busy timeout to 5000
  milliseconds, because this ADR asks for one and names no value. Pass every
  setting in the file address of the database, so a connection that the pool
  replaces keeps WAL mode, `synchronous=FULL`, foreign keys, and the busy
  timeout. Read each setting back after the open and fail when one differs,
  because SQLite leaves WAL mode unchanged without an error on a filesystem
  that cannot share memory, and the server must not run with weaker durability
  than this ADR asks. Allow one open connection, so every transaction waits for
  the one before it. Make the path of the database absolute before building the
  address, because a file address with a relative path reads its first segment
  as a host. Reject an empty path and a path with a NUL byte, because SQLite
  cuts the name at the NUL and opens a different file. Leave `BEGIN IMMEDIATE`
  to the activation code, which asks for it per transaction. Leave the data
  directory, the lock file, and the file modes to later steps.
- 2026-10-31. Clarified how the server locks the data directory while building
  `internal/storage`, without changing a decision. Take an exclusive `flock` on
  the `lock` file without waiting, so a second process fails at once with an
  error that names the directory and asks the operator to stop the other
  process or choose another directory. Write nothing to the file, because the
  lock is the `flock` and not the content. The operating system drops the lock
  when the process exits, so a crash leaves no stale lock. Open the directory
  with `os.Root` and create the file with mode 0600. Never create the data
  directory, because a mistyped path would make a new empty one in the wrong
  place, and fail with an error that says to create it. Reject an empty path,
  and reject a path that is not a directory by checking the path with
  `os.Stat`, because `os.OpenRoot` reports a file with plain text that no code
  can match. Follow a symlinked data directory, which operators use to reach
  another volume. Look at the `lock` entry with `Lstat` before the open and
  reject a symlink, a directory, a named pipe, or any other entry that is not a
  regular file. `os.Root` follows a symlink that stays inside the directory, so
  without the check the lock would sit on the wrong file, and the open would
  create the target of a dangling symlink. Leave the mode of an existing lock
  file alone. Accept that a symlink put in place of the entry between the check
  and the open is not caught. It needs write access to the data directory, and
  anyone with that access can change the database too. A later step makes the
  directory private by refusing one that group or others can access. Support
  unix platforms only, because the lock uses `syscall.Flock`, which covers the
  Linux and macOS server platforms of this ADR.
- 2026-10-31. Clarified how the server judges the access to the data directory
  while building `internal/storage`, without changing a decision. Refuse a data
  directory with any permission bit of group or other set, so a mode such as
  0755 or 0750 fails and 0700 passes. The directory holds the database and the
  lock file, and the symlink swap that the entry on the lock accepts needs write
  access to it, so a private directory closes that gap. Run the check after the
  checks for an empty path, a missing directory, and a path that is not a
  directory, so the error names the first problem. Run it before the open, so a
  refused directory never gets a lock file. Read the mode with `os.Stat`, so a
  symlinked data directory is judged by its target, which is the directory that
  the server uses. Put the `chmod -- 700` command with the quoted path in the
  error on one line, and show the quoted path with no command when the path has
  a control or unprintable character, as the user config error does. Check the
  mode and not the owner, so a directory owned by another user passes when its
  mode is 0700. Such a directory is outside what this rule protects, and an
  owner check would break some container and mounted volume setups. A later ADR
  entry can add it. Do not check the opened directory again. A swap between the
  check and the open needs write access to a parent of the path, and anyone with
  that access can replace the whole data directory, so a second check would not
  stop that attacker. It would add a path that no test can force without a seam.
  The check does not cover the modes of the files and directories that a later
  step creates inside the data directory.
- 2026-11-01. Clarified how the server makes the layout of the data directory
  while building `internal/storage`, without changing a decision. Make
  `projects/`, `quarantine/`, and `pruning/` inside the data directory, and
  leave `staging/` and `releases/` to the step that creates a project, because
  they sit under `projects/<project-id>/` and nothing can name a project yet.
  Give each directory mode 0700 and set it with `chmod` after the `mkdir`,
  because the umask limits a new directory and the mode must not depend on it.
  Never create the data directory, as the entry on the lock says, and apply the
  same checks for an empty, missing, or non-directory path and for a mode that
  group or other can use, in that order. Check every existing entry before the
  first `mkdir`, so a refused layout creates nothing. Look at each entry with
  `Lstat` and refuse a symlink, a file, or any other entry that is not a
  directory. `os.Root` follows a symlink that stays inside the directory, and a
  symlinked `pruning/` or `projects/` could lead to another volume, which would
  break the atomic rename that this ADR needs between them. Refuse an existing
  directory that group or other can use with the `chmod -- 700` error and do not
  fix it, because a silent fix would hide that something else changed the mode.
  Leave an existing directory and its content alone, so a restart is safe. Run
  the layout after the lock, and run recovery after the database opens, so the
  start order is the lock, the layout, the database, and then recovery.
  `ship8td serve` owns that order, and nothing calls the layout yet. This entry
  does not check that `pruning/` and `projects/` share a filesystem, and it does
  not set the mode of `ship8t.db` and its journal files. Each is its own step.
- 2026-11-01. Clarified how the server keeps the database files private while
  building `internal/storage`, without changing a decision. The database holds
  token hashes, so give `ship8t.db`, `ship8t.db-wal`, and `ship8t.db-shm` mode
  0600, as the rule of `docs/go-quality.md` for a file that holds secrets asks.
  Create a missing `ship8t.db` empty with mode 0600 through `os.Root` before
  SQLite opens it, and set the mode with `chmod` after the create. The driver
  creates the file with mode 0644 under the umask, and a umask can clear owner
  bits. SQLite gives `-wal` and `-shm` the mode of the main file, so one private
  file covers all three. A probe of `modernc.org/sqlite` v1.61.1 on Go 1.26.0
  and 1.27.3 showed both. Create the file with `O_EXCL`, so a symlink at the
  path is not followed. Look at `ship8t.db`, `-wal`, and `-shm` with `Lstat`
  before the create, and check all three before making any, so a refused open
  creates nothing. Refuse an entry that is not a regular file, because SQLite
  follows a symlink at the path and would create the target of a dangling one.
  Refuse an existing file with any permission bit of group or other set. Put the
  `chmod -- 600` command with the quoted path of that file in the error on one
  line, and show the quoted path with no command when the path has a control or
  unprintable character, as the errors for the data directory do. Refuse and do
  not fix, because a silent fix hides that something else changed the mode, and
  others may have read the file already. Check `-wal` and `-shm` as well,
  because a crash leaves them with the mode of an earlier run, and a probe
  showed that a leftover `-wal` with content kept mode 0644 while the main file
  was 0600. Refuse a loose one whatever its size, because the check reads
  neither size nor content, and no release exists yet that could have left one.
  Check the mode and not the owner, as for the data directory. Do not check the
  files again between the check and the create. A swap needs write access to the
  data directory, which the access rule keeps private, and `O_EXCL` stops the
  create from following a symlink. No test can force that swap without a seam.
  Open the parent directory with `os.Root`, so a missing parent directory fails
  with the operating system error and no longer with the SQLite text. This entry
  does not check that `pruning/` and `projects/` share a filesystem, and it does
  not check the mode of a file again after the open.
- 2026-11-01. Clarified how the server creates the schema of the database while
  building `internal/storage`, without changing a decision. Mark a database made
  by this code with `user_version` 1, because no ADR names a schema version.
  Read the version and the contents of the database and then set the version and
  create the tables, all in one plain transaction, so a failed statement leaves
  version 0 and no tables. Set the version before the statements, so a failure
  shows that the rollback undoes it too. Use a plain transaction and not `BEGIN
  IMMEDIATE`, which stays with the activation code, because the lock and the
  single connection leave no other writer. Create version 1 in a database with
  version 0 and no tables, and do not count a table whose name starts with
  `sqlite_`, because a dropped `AUTOINCREMENT` table leaves `sqlite_sequence`
  behind. Accept version 1 and change nothing. Refuse a database that has tables
  and version 0, and a database with any other version, a negative one included.
  Name the version found and the version wanted in the error, say what the
  operator can do, and leave the database as it was. Run the guard after `Open`
  and not inside it, so `Open` keeps opening a database that has no version.
  Build no migration framework, because the first release has one schema. Until
  that release, version 1 grows in place with each table, so a development
  database from an earlier step still says version 1 and the operator deletes
  it. Trust a database at version 1 and do not read its tables to check their
  shape. The first schema holds `projects` and `deployments`, both `STRICT`.
  `projects` has the ID as its primary key, and a CHECK holds the slug rule of
  this ADR, which is 1 to 63 bytes of lowercase letters, digits, and hyphens
  with no leading or trailing hyphen. The CHECK compares the length in bytes
  with the length in characters, because SQLite text functions stop at a NUL
  byte and would let a name with one pass. The CHECK is a backstop. The code
  that creates a project still validates the ID before it builds a path.
  `deployments` has the ID as its primary key, the project ID as a foreign key
  with `ON DELETE RESTRICT`, so no project can be deleted while a deployment of
  it exists and history stays whole, a state that a CHECK limits to the seven
  states of ADR 0002, and the manifest as text. The state CHECK keeps a state
  that was read wrongly from persisting. Put no CHECK on the deployment ID,
  because no ADR gives its shape. Put no CHECK on the size of the manifest,
  because `Decode` owns the limit of ADR 0004. Leave the active pointer, the
  timestamps, the rows for files, the tokens, the activation log, and the
  idempotency keys to the code that needs each one. The pointer needs a foreign
  key to `deployments`, which would make a cycle with this table. Accept three
  limits that the tests cannot show. A crash between the commit and a version
  set outside the transaction would be the only way to see that the version
  belongs in the transaction, and no seam forces that crash. A second writer
  between the read and the create would be the only way to see that the read
  belongs in the transaction, and the lock and the single connection leave none.
  A STRICT table already makes a primary key column NOT NULL, so the keyword on
  the ID of `deployments` is redundant and no test can tell it apart.
- 2026-11-01. Clarified how the server checks that the layout shares a
  filesystem while building `internal/storage`, without changing a decision.
  This ADR needs `projects/` and `pruning/` on one filesystem, so that moving a
  release into `pruning/` is an atomic rename. The layout entry of this date
  left the check to a later step, and this entry adds it. Check it with a
  separate function, `CheckLayoutFilesystem`, that the caller runs after the
  layout, so the layout and its tests stay as they are. Apply the checks of the
  layout first, which are an empty path, a missing directory, a path that is not
  a directory, and a mode that group or other can use, in that order. Look at
  `projects/` and `pruning/` with `Lstat` through `os.Root`. Refuse an entry
  that is not a directory, and refuse one that group or other can use, with the
  errors of the layout, so a layout that the layout function would refuse never
  passes. When either directory is missing, fail with an error that says to
  create the layout first. Compare the device of the two directories and refuse
  when they differ, with one line that names both directories and says to put
  them on one filesystem. Quote each path, so an unprintable one stays on one
  line, and never print a device ID. When a directory has no stat result, fail
  with its own error that names the directory and the type found, so an unknown
  device never counts as the same. Change nothing, so the check never fixes a
  layout. The start order is the lock, the layout, the filesystem check, the
  database, and then recovery, and `ship8td serve` owns it. Nothing calls the
  check yet. Accept these limits. A bind mount of the same filesystem can show
  one device ID and still fail a rename with `EXDEV`, so the comparison can miss
  it, and no test without root can show either case. A mount below
  `projects/<id>`, such as at `releases`, cannot exist before a project does, so
  the step that creates a project checks it. The check does not look at
  `quarantine/`, because this ADR does not ask for it to share the filesystem,
  and the recovery design decides whether it should. The check looks at each
  entry twice, once for its type and mode and once for its device, and a swap
  between the two needs write access to the private data directory, which is the
  same limit as the other checks. Only a real mount makes two entries differ, so
  the refusal is tested through the unexported function that compares two
  directories, and no test catches a caller that drops its result or swaps the
  two paths. Replacing `Lstat` with `Stat` changes nothing, because the check of
  the type refuses a symlink first.
