// Package storage will own every write to the data directory of the server, as
// ADR 0003 sets out. It holds only tests until that code exists. Each test
// skips until the code that it needs exists, so the tests show which
// interruption points are still unchecked.
package storage
