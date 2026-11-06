# Current state

Update this file in the same change that adds behavior. The
[README](../README.md) holds the project overview, the structure, and the
roadmap.

Only the following exists in the code today.

- `ship8t` and `ship8td` each accept `version`, which prints the build version,
  and `help`, which prints the usage text. `--help` and `-h` also print it.
- Any other command prints the usage text to standard error and exits with
  code 2.
- The version comes from a build-time stamp, then from the Go module build info,
  then falls back to `dev`.
- `internal/manifest` validates and decodes deployment manifests. A manifest
  lists every file of a deployment with its path, size, and SHA-256 checksum, as
  ADR 0004 defines. A manifest also carries `manifest_version` and a `serve`
  object with `spa_fallback`, which is off by default.
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
  - It rejects a manifest that lists one checksum with two different sizes,
    because a checksum names one content and so one size. The error names the
    position of each of the two files and their sizes. Two files with one
    checksum and one size are accepted, and so are two empty files. The check
    runs after the path checks, so a manifest with both problems reports the
    path first.
  - `Validate` applies the same rules and limits to a manifest that code builds
    in memory.
  - `Blobs` returns each distinct checksum of a manifest once with its size, as
    `Blob` values sorted by checksum, so the answer does not depend on the order
    of the files. This is the list that step 1 of the ADR 0004 upload flow calls
    `upload.missing`. Files with one checksum share one entry, and an empty file
    has an entry. It expects a manifest that passed validation and has no error
    return. Nothing calls it yet, because the first caller is the upload code.
  - `internal/scan` and `internal/storage` use it. No command uses it yet.
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
    content. It gives each file its own placeholder checksum for that first
    check, because the manifest rules allow one size for each checksum and the
    sizes are not final yet.
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
    `chmod -- 600` command on one line. It omits the command and quotes the path
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
  checks a move between two of them. `internal/storage` uses its state
  constants and calls `Transition` when it cancels or expires a deployment.
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
- `internal/storage` opens the SQLite database, creates its first tables, locks
  the data directory, makes its directories, adds a project with its
  directories, adds a deployment from a manifest, cancels an unfinished
  deployment, expires idle deployments, and holds skipped tests for the rest of
  storage. No command or server code uses it yet.
  - `Open` takes the path of the database file, creates it when it is missing,
    and returns a `Database`. `Close` closes it. The driver is the pure Go
    `modernc.org/sqlite`, so no C toolchain is needed.
  - `Open` creates a missing database file empty with mode 0600 before SQLite
    opens it, so the umask cannot loosen it. It sets the mode with `chmod`, so a
    umask that clears the owner bits does not change it either. SQLite gives
    `-wal` and `-shm` the mode of the main file, so all three are private. It
    opens the parent directory through `os.Root`, so a missing parent directory
    fails with the operating system error.
  - `Open` refuses an existing database file, `-wal`, or `-shm` that group or
    other can use, and does not fix it. `Open` takes any path, and `ship8t.db`
    is only the name that the data directory will use. The error holds the
    `chmod -- 600` command with the quoted path of that file on one line. It
    omits the command and quotes the path when the path has a control or
    unprintable character. A loose `-wal` or `-shm` is refused whatever its
    size, because a crash can leave one with the mode of an earlier run.
  - `Open` also refuses a symlink and any other entry that is not a regular
    file at any of the three names, because SQLite follows a symlink and would
    create its target. It checks the entries with `Lstat` before it creates the
    file, so a refused open creates nothing. It checks the mode and not the
    owner, and it does not check the files again after the open.
  - `Open` sets the journal mode to WAL, `synchronous` to `FULL`, foreign keys
    on, and a busy timeout of 5 seconds, and it uses one connection, so
    transactions run one at a time. The settings are part of the connection
    address, so a connection that the pool opens later has them too.
  - `Open` reads each setting back and fails when one differs, because SQLite
    can leave WAL mode off without an error on some filesystems.
  - `Open` makes a relative path absolute. It rejects an empty path and a path
    with a NUL byte, because SQLite would cut the name there and open another
    file.
  - `Open` does not create the tables, the data directory, or the lock.
  - `EnsureSchema` makes the database ready. Call it after `Open`. It runs in
    one plain transaction that reads the schema version and the contents of the
    database and then creates the tables. A database with version 0 and no
    tables gets version 1 and the tables. A table that SQLite keeps for itself,
    such as `sqlite_sequence`, does not count.
  - `EnsureSchema` accepts a database with version 1 and changes nothing. It
    refuses a database that has tables and version 0, and a database with any
    other version, a negative one included. The error names the version found
    and the version wanted and says what to do. A refused database stays as it
    was, and a failed creation leaves version 0 and no tables.
  - Version 1 has two tables, both `STRICT`. `projects` holds the ID, which a
    CHECK limits to a slug of 1 to 63 bytes of lowercase letters, digits, and
    hyphens with no leading or trailing hyphen. The CHECK rejects a NUL byte.
  - `deployments` holds the ID, the project ID, the state, the manifest as
    text, and `updated_at`, the time of the last activity in Unix seconds. The
    project ID is a foreign key to `projects` with `ON DELETE RESTRICT`, so a
    project with a deployment cannot be deleted. A CHECK limits the state to the
    seven states of ADR 0002.
  - A CHECK limits the deployment ID to 32 bytes of lowercase hexadecimal
    characters and rejects a NUL byte, because the ID becomes a directory name.
    The ID holds the creation time, so there is no `created_at`. No rule limits
    the size of the manifest, because `Decode` owns that limit.
  - The tables have no active pointer, and there are no tables for files,
    tokens, the activation log, or idempotency keys yet. `CreateProject` writes
    to `projects`, `CreateDeployment` writes to `deployments`, and
    `CancelDeployment` and `ExpireIdleDeployments` change rows of `deployments`.
    They are the only code that updates `updated_at` after the insert.
  - Version 1 grows with each table until the first release. A development
    database from an earlier step still says version 1, so delete it by hand.
    There is no migration.
  - `LockDataDirectory` takes the exclusive lock on a data directory and returns
    a `DataDirectoryLock`. `Release` gives the lock up. A server holds the lock
    for its whole life, so two server processes never share a directory.
  - The lock is a `flock` on the file `lock`, which the code opens through
    `os.Root` and creates with mode 0600. A second attempt fails at once with an
    error that names the directory and asks to stop the other process or choose
    another directory. The file holds nothing, and the operating system drops
    the lock when the process exits, so a crash leaves no stale lock.
  - The lock rejects an empty path, a missing directory, and a path that is not
    a directory. It never creates the directory, so a mistyped path does not
    make a new empty one. It follows a symlinked directory.
  - The lock rejects a `lock` entry that is not a regular file, such as a
    symlink, a directory, or a named pipe. It checks the entry with `Lstat`,
    because `os.Root` follows a symlink that stays inside the directory. An
    existing lock file keeps its mode. A symlink put in place of the entry
    between the check and the open is not caught.
  - The lock rejects a data directory that group or other can use, so a mode
    such as 0755 fails and 0700 passes. The error holds the `chmod -- 700`
    command with the quoted path on one line. It omits the command and quotes
    the path when the path has a control or unprintable character. The check
    runs after the checks for an empty, missing, or non-directory path and
    before the lock file is created, so a refused directory gets no lock file.
    It reads the mode of the directory that the path leads to, so a symlinked
    directory is judged by its target.
  - The access check looks at the mode and not at the owner, so a directory
    owned by another user passes when its mode is 0700. It does not check the
    opened directory again. It does not cover the modes of the database files,
    which `Open` checks, or of the directories that later steps create inside
    the data directory.
  - The `internal/storage` package builds on unix platforms only, because the
    lock uses `syscall.Flock` and the filesystem check uses `syscall.Stat_t`.
    Nothing calls the lock yet.
  - `CreateDataDirectoryLayout` makes `projects/`, `quarantine/`, and
    `pruning/` inside an existing data directory with mode 0700. It sets the
    mode with `chmod`, so the umask does not change it. `CreateProject` makes
    the directories of a project.
  - It applies the checks of the lock first, so it rejects an empty, missing, or
    non-directory path and a data directory that group or other can use. It
    never creates the data directory. Call it after `LockDataDirectory`.
  - It checks every existing entry before it makes the first directory, so a
    refused layout creates nothing. It rejects an entry that is not a directory,
    such as a symlink, a file, or a named pipe, because a symlink could lead to
    another volume. It rejects an existing directory that group or other can
    use, with the `chmod -- 700` command, and does not fix it. As for the data
    directory, it omits the command and quotes the path when the path has a
    control or unprintable character.
  - It leaves an existing directory and its content alone, so a repeat is safe.
    It does not check that `pruning/` and `projects/` share a filesystem, which
    `CheckLayoutFilesystem` does. Nothing calls it yet.
  - `CheckLayoutFilesystem` rejects a data directory whose `projects/` and
    `pruning/` are on different filesystems, because ADR 0003 moves a release
    between them with an atomic rename. Call it after
    `CreateDataDirectoryLayout`. It changes nothing and fixes nothing. Nothing
    calls it yet.
  - It applies the checks of the layout first, so it rejects an empty, missing,
    or non-directory path and a data directory that group or other can use. It
    looks at `projects/` and `pruning/` with `Lstat`. It rejects an entry that
    is not a directory or that group or other can use, with the errors of the
    layout, so a symlink never stands for its target. It rejects a missing
    entry with an error that says to create the layout first.
  - It compares the device of the two directories. The error for two devices
    names both directories on one line and says to put them on one filesystem.
    It quotes each path, so an unprintable path stays on one line, and it never
    prints a device ID. A directory with no stat result gets its own error, so
    an unknown device never counts as the same.
  - It does not catch a bind mount of the same filesystem, which can show one
    device and still fail a rename with `EXDEV`. It does not look at
    `quarantine/` or at a mount below `projects/<id>`, which `CreateProject`
    checks for the existing directories of a project. No test can make a real
    mount, so the refusal is tested through an unexported function that takes
    two directories.
  - `CreateProject` adds a project to the `projects` table and makes its
    directories in the data directory, in one plain transaction that commits
    last. Call it after `EnsureSchema` and `CreateDataDirectoryLayout`. It takes
    the context, the data directory, and the project ID. Nothing calls it yet.
  - It rejects an ID that is not a slug, which is 1 to 63 bytes of lowercase
    letters, digits, and hyphens with no leading or trailing hyphen, before it
    builds any path. The error names the rule and never repeats an oversized
    value. The table holds the same rule as a backstop, and `internal/config`
    holds a copy for the client.
  - It makes `projects/<id>` with `staging` and `releases` inside it and mode
    0700, set with `chmod` after each `mkdir`. It needs `projects/` and
    `pruning/` to exist, and it says to create the layout first when one is
    missing. It applies the checks of the data directory and of the layout, and
    it checks every existing entry before the first `mkdir`, so a refused
    project creates nothing. It rejects an entry that is not a directory or that
    group or other can use, with the errors of the layout, and it never follows
    a symlink. It adopts a real directory with mode 0700 and leaves its content
    alone.
  - It rejects a project whose existing directories are on another filesystem
    than `pruning/`, with the error of the filesystem check. It compares
    `projects/` and each existing entry of the project with `pruning/` before
    the first `mkdir`, and a new directory is on the filesystem of its parent.
    It does not catch a mount placed after the project was made or a bind mount
    of the same filesystem, and no test can make a mount.
  - It makes the directories first and then inserts the row with `ON CONFLICT DO
    NOTHING`, so an existing project is accepted and a repeat is safe. A repeat
    makes any directory of the project that is missing. A failure rolls back and
    leaves no row. A failed insert or a crash before the commit can leave the
    empty directories of a project that has no row, and the next call adopts
    them.
  - The data directory is a parameter that nothing ties to the database file, so
    the caller must pass the one that holds `ship8t.db`.
  - `CreateDeployment` adds a deployment in the `created` state to a project and
    returns its ID. Call it after `EnsureSchema` and `CreateProject`. It takes
    the context, the project ID, a decoded manifest, the manifest limits, and
    the number of unfinished deployments that a project may hold. The manifest
    must come from `manifest.Decode` or be built by code that validates it, and
    it must not be nil. Nothing calls it yet.
  - The ID has 32 lowercase hexadecimal characters. The first 12 hold the
    milliseconds since the Unix epoch and the last 20 hold 80 random bits from
    `crypto/rand`, so IDs sort by the millisecond that made them and the order
    inside one millisecond is random. A clock before 1970 or after the largest
    time that 12 characters hold fails with `system clock is outside the
    supported range, so check the date of the server`.
  - It checks the project ID against the slug rule before anything else, and it
    validates the manifest again against the limits that the caller passes,
    because the server never trusts the caller. It stores the canonical JSON of
    the manifest. An error of the manifest wraps `manifest.ErrLimitExceeded` or
    `manifest.ErrUnsupportedVersion` where those apply, so a caller can tell the
    kinds apart.
  - One plain transaction checks that the project exists, counts the project's
    deployments in the `created` and `uploading` states, and inserts the row. A
    missing project fails with `project "<id>" does not exist, so create it
    first`. A project at the limit fails with `project "<id>" has <count>
    unfinished deployments and the limit is <max>, so finish or cancel one
    first`. A limit of zero or less refuses every call, and the code that reads
    the setting must check it. A refusal leaves no row.
  - It makes no directory, because `created` means that no content has been
    accepted, and the first upload will make `staging/<id>`. A repeat of a call
    makes a second deployment, because the idempotency key of ADR 0004 does not
    exist yet. `ExpireIdleDeployments` expires idle ones, and nothing runs it
    yet.
  - No test forces a failed commit or a clock outside the range inside
    `CreateDeployment`, because each needs a seam. A test cannot tell which step
    of the transaction used a cancelled context, because the first step fails.
  - `CancelDeployment` moves an unfinished deployment to the `cancelled` state.
    Call it after `CreateDeployment`. It takes the context and the deployment
    ID. Nothing calls it yet.
  - It checks the shape of the ID before it opens a transaction. The ID must
    have 32 lowercase hexadecimal characters, the shape that `CreateDeployment`
    makes. The error names the rule and never repeats the value. The table holds
    the same rule as a backstop, and a test compares the two.
  - One plain transaction reads the state, asks `Transition` for the move to
    `cancelled`, and updates the row. A `created` or `uploading` deployment
    becomes `cancelled`, and its `updated_at` is set to the time of the change
    in Unix seconds. A deployment that is already `cancelled` succeeds and its
    row stays as it was, `updated_at` included, so a repeated request is safe.
  - Any other state fails with an error that wraps `ErrStateConflict`, such as
    `cancel deployment: state conflict: a deployment cannot move from "ready" to
    "cancelled"`, and leaves the row as it was. A deployment that does not exist
    fails with `deployment "<id>" does not exist`.
  - It changes the row only and touches no directory, because recovery deletes
    the staging directory of a terminal deployment. A cancelled deployment no
    longer counts toward the limit of unfinished deployments, so a project at
    the limit can create another one.
  - No test forces a failed commit or a failed rollback inside
    `CancelDeployment`, because each needs a seam. A test cannot tell which step
    of the transaction used a cancelled context, because the first step fails.
  - `ExpireIdleDeployments` moves every unfinished deployment that has been idle
    for the timeout to the `expired` state and returns their IDs in order. Call
    it after `EnsureSchema`. It takes the context and the idle timeout. Nothing
    calls it yet, and nothing runs it on a schedule.
  - It rejects a timeout below one second with an error that names the value,
    because a timeout of zero or less would expire every unfinished deployment.
    This differs from the limit of unfinished deployments, where zero or less
    refuses every create.
  - A `created` or `uploading` deployment expires when its `updated_at` is at
    least the timeout old, counted in whole seconds, so a deployment that has
    been idle for exactly the timeout expires. The row gets the state `expired`
    and an `updated_at` set to the time of the sweep. A deployment in any other
    state is left alone, an `expired` one included, so a repeat writes nothing.
  - One plain transaction selects the candidates and asks `Transition` for each
    move, and a failure rolls back the whole sweep. It changes rows only and
    touches no directory. Removing `staging/<id>` on expiry and on cancel, and
    refreshing `updated_at` when a file is accepted, are owed to the upload code
    that makes the directory. An expired deployment no longer counts toward the
    limit of unfinished deployments.
  - No test forces a failed commit or a failed rollback inside the sweep,
    because each needs a seam. The state filter lets only `created` and
    `uploading` rows through, so `Transition` never refuses one, and no test
    shows that the sweep calls it.
  - The tests for `Open`, the lock, the access rule, the layout, the
    filesystem check, project creation, deployment creation, cancelling a
    deployment, and expiring idle deployments run and pass.
    Only the cases listed below skip until the code they need exists, so none of
    them checks anything today.
  - `TestRecoveryAfterInterruption` lists ten cases from ADR 0003. They are a
    crash before the `ready` commit, the staging directories of dead and live
    deployments, an unknown release directory, a missing release directory, an
    active release whose directory sits in `pruning/`, leftover pruning
    entries, a crash during activation, and two interruptions of a prune.
  - `TestConcurrentActivation` lists five races between activation operations
    of one project. `TestPruneRefusesActiveRelease` lists two attempts to prune
    the active release.
  - Each case names the code it needs and the check to make once that code
    exists.
