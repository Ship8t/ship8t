package storage

import (
	"sort"
	"strings"
	"testing"
	"time"
)

// maxMillisecondsTest is the largest time that the 12 hexadecimal characters
// of an ID can hold, which is 2^48 - 1 milliseconds. It is written out so a
// change of the limit in the code shows here.
const maxMillisecondsTest = 281474976710655

func TestNewDeploymentIDStartsWithTheTime(t *testing.T) {
	tests := []struct {
		name   string
		now    time.Time
		prefix string
	}{
		{name: "epoch", now: time.UnixMilli(0), prefix: "000000000000"},
		{name: "one millisecond", now: time.UnixMilli(1), prefix: "000000000001"},
		{name: "a known time", now: time.UnixMilli(1_700_000_000_123), prefix: "018bcfe5687b"},
		{name: "last digit carries", now: time.UnixMilli(0xfff), prefix: "000000000fff"},
		{name: "first carry", now: time.UnixMilli(0x1000), prefix: "000000001000"},
		{name: "largest time", now: time.UnixMilli(maxMillisecondsTest), prefix: "ffffffffffff"},
		{name: "drops the sub millisecond part", now: time.Unix(0, 1_999_999), prefix: "000000000001"},
		{name: "ignores the time zone", now: time.UnixMilli(255).In(time.FixedZone("far", 14*3600)), prefix: "0000000000ff"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, err := newDeploymentID(test.now)
			if err != nil {
				t.Fatalf("newDeploymentID() error = %v, want nil", err)
			}
			if len(id) != 32 {
				t.Errorf("newDeploymentID() = %q, want 32 characters", id)
			}
			if !strings.HasPrefix(id, test.prefix) {
				t.Errorf("newDeploymentID() = %q, want prefix %q", id, test.prefix)
			}
			for _, character := range id {
				isDigit := character >= '0' && character <= '9'
				isLowercaseHex := character >= 'a' && character <= 'f'
				if !isDigit && !isLowercaseHex {
					t.Errorf("newDeploymentID() = %q, want only lowercase hexadecimal characters", id)
					break
				}
			}
		})
	}
}

func TestNewDeploymentIDSortsByTime(t *testing.T) {
	// Step across several carries of the hexadecimal time, from 0xff0 to 0x1010.
	var ids []string
	for milliseconds := int64(0xff0); milliseconds <= 0x1010; milliseconds++ {
		id, err := newDeploymentID(time.UnixMilli(milliseconds))
		if err != nil {
			t.Fatalf("newDeploymentID(%d) error = %v, want nil", milliseconds, err)
		}
		ids = append(ids, id)
	}

	if !sort.StringsAreSorted(ids) {
		t.Errorf("IDs of rising times are not in rising order")
	}
}

func TestNewDeploymentIDIsRandomWithinAMillisecond(t *testing.T) {
	const count = 1000
	now := time.UnixMilli(1_700_000_000_123)
	seen := make(map[string]bool, count)

	for range count {
		id, err := newDeploymentID(now)
		if err != nil {
			t.Fatalf("newDeploymentID() error = %v, want nil", err)
		}
		seen[id] = true
	}

	if len(seen) != count {
		t.Errorf("distinct IDs for one millisecond = %d, want %d", len(seen), count)
	}
}

func TestNewDeploymentIDUsesEveryByteOfTheRandomPart(t *testing.T) {
	// Expect each of the 20 random characters to take more than one value, which
	// a constant or a shorter random part would not.
	now := time.UnixMilli(1_700_000_000_123)
	values := make([]map[byte]bool, 20)
	for index := range values {
		values[index] = map[byte]bool{}
	}

	for range 200 {
		id, err := newDeploymentID(now)
		if err != nil {
			t.Fatalf("newDeploymentID() error = %v, want nil", err)
		}
		if len(id) != 32 {
			t.Fatalf("newDeploymentID() = %q, want 32 characters", id)
		}
		for index := range values {
			values[index][id[12+index]] = true
		}
	}

	for index, seen := range values {
		if len(seen) < 2 {
			t.Errorf("random character %d took %d values in 200 IDs, want at least 2", index, len(seen))
		}
	}
}

func TestNewDeploymentIDRejectsTimesOutOfRange(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
	}{
		{name: "one millisecond before the epoch", now: time.UnixMilli(-1)},
		{name: "one second before the epoch", now: time.Unix(-1, 0)},
		{name: "zero time value", now: time.Time{}},
		{name: "one millisecond after the largest time", now: time.UnixMilli(maxMillisecondsTest + 1)},
		{name: "year 10890", now: time.Date(10890, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, err := newDeploymentID(test.now)
			want := "system clock is outside the supported range, so check the date of the server"
			if err == nil || err.Error() != want {
				t.Errorf("newDeploymentID() error = %v, want %q", err, want)
			}
			if id != "" {
				t.Errorf("newDeploymentID() = %q, want no ID with an error", id)
			}
		})
	}
}

func TestNewDeploymentIDSatisfiesTheTable(t *testing.T) {
	database := ensureTestSchema(t)

	for _, milliseconds := range []int64{0, 1, 1_700_000_000_123, maxMillisecondsTest} {
		id, err := newDeploymentID(time.UnixMilli(milliseconds))
		if err != nil {
			t.Fatalf("newDeploymentID(%d) error = %v, want nil", milliseconds, err)
		}
		if err := insertDeploymentTest(t.Context(), database, id, "my-site", "created", "{}"); err != nil {
			t.Errorf("insert of the ID %q for %d milliseconds error = %v, want nil", id, milliseconds, err)
		}
	}
}
