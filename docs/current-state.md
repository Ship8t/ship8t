# Current state

Update this file in the same change that adds behavior. The
[README](../README.md) holds the project overview, the structure, and the
roadmap.

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
- `internal/storage` opens the SQLite database, creates its first tables, locks
  the data directory, and holds skipped tests for the rest of storage. No
  command or server code uses it yet.
  - `Open` takes the path of the database file, creates it when it is missing,
    and returns a `Database`. `Close` closes it. The driver is the pure Go
    `modernc.org/sqlite`, so no C toolchain is needed.
  - `Open` creates a missing database file empty with mode 0600 before SQLite
    opens it, so the umask cannot loosen it. It sets the mode with `chmod`, so a
    umask that clears the owner bits does not change it either. SQLite gives
    `-wal` and `-shm` the mode of the main file, so all three are private. It
    opens the parent directory through `os.Root`, so a missing parent directory
    fails with the operating system error.
  - `Open` refuses an existing `ship8t.db`, `-wal`, or `-shm` that group or
    other can use, and does not fix it. The error holds the `chmod -- 600`
    command with the quoted path of that file on one line. It omits the command
    and quotes the path when the path has a control or unprintable character. A
    loose `-wal` or `-shm` is refused whatever its size, because a crash can
    leave one with the mode of an earlier run.
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
  - `deployments` holds the ID, the project ID, the state, and the manifest as
    text. The project ID is a foreign key to `projects` with `ON DELETE
    RESTRICT`, so a project with a deployment cannot be deleted. A CHECK limits
    the state to the seven states of ADR 0002.
  - No rule limits the deployment ID or the size of the manifest. The tables
    have no timestamps and no active pointer, and there are no tables for files,
    tokens, the activation log, or idempotency keys yet. Nothing writes a row.
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
  - The lock builds on unix platforms only, because it uses `syscall.Flock`.
    Nothing calls it yet.
  - `CreateDataDirectoryLayout` makes `projects/`, `quarantine/`, and
    `pruning/` inside an existing data directory with mode 0700. It sets the
    mode with `chmod`, so the umask does not change it. The directories of a
    project come with the project, and nothing creates them yet.
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
    `quarantine/` or at a mount below `projects/<id>`, which the step that
    creates a project checks. No test can make a real mount, so the refusal is
    tested through an unexported function that takes two directories.
  - The tests for `Open`, the lock, the access rule, the layout, and the
    filesystem check run and pass. Only the cases listed below skip until the
    code they need exists, so none of them checks anything today.
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
