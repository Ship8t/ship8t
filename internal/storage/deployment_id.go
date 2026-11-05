package storage

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

// maxDeploymentIDMilliseconds is the largest time that the 12 hexadecimal
// characters at the start of a deployment ID can hold, which is 2^48 - 1
// milliseconds since the Unix epoch.
const maxDeploymentIDMilliseconds = 1<<48 - 1

// errClockOutOfRange reports a time that no deployment ID can start with.
var errClockOutOfRange = errors.New("system clock is outside the supported range, so check the date of the server")

// newDeploymentID returns a time-sortable random ID of 32 lowercase hexadecimal
// characters, as ADR 0003 asks. It holds the milliseconds of now in 12
// characters and 80 random bits in 20, so IDs sort by the millisecond that made
// them and the order inside one millisecond is random. It fails when now falls
// before 1970 or after the largest time of the 12 characters, because the
// deployments table would refuse such an ID without saying why.
func newDeploymentID(now time.Time) (string, error) {
	milliseconds := now.UnixMilli()
	if milliseconds < 0 || milliseconds > maxDeploymentIDMilliseconds {
		return "", errClockOutOfRange
	}
	var random [10]byte
	// Read never returns an error since Go 1.24, and it ends the program when the
	// system has no randomness, so there is no failure to handle.
	_, _ = rand.Read(random[:])
	return fmt.Sprintf("%012x%x", milliseconds, random), nil
}
