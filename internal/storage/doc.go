// Package storage will own every write to the data directory of the server, as
// ADR 0003 sets out. It opens the SQLite database today. Each test of an
// interruption point skips until the code that it needs exists, so the skipped
// tests show which interruption points are still unchecked.
package storage
