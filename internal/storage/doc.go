// Package storage will own every write to the data directory of the server, as
// ADR 0003 sets out. Today it opens the SQLite database, creates its first
// tables, locks the data directory, and makes the top level directories. Each
// test of an interruption point skips until the code that it needs exists, so
// the skipped tests show which interruption points are still unchecked.
package storage
