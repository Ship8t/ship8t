package storage

import "fmt"

// maxProjectIDLength is the longest project ID, the length of a DNS label.
const maxProjectIDLength = 63

// validateProjectID rejects an ID that is not a slug of ADR 0003, which is 1 to
// 63 bytes of lowercase letters, digits, and hyphens with no leading or trailing
// hyphen. It runs before any path is built from the ID. The table of projects
// holds the same rule as a backstop, and internal/config holds a copy for the
// client.
func validateProjectID(id string) error {
	// Check the length first, so an error never echoes an oversized value.
	if len(id) < 1 || len(id) > maxProjectIDLength {
		return fmt.Errorf("project ID must be 1 to %d characters, got %d", maxProjectIDLength, len(id))
	}
	for index := 0; index < len(id); index++ {
		character := id[index]
		isLowercaseLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		if !isLowercaseLetter && !isDigit && character != '-' {
			return fmt.Errorf("project ID %q must use only lowercase letters, digits, and hyphens", id)
		}
	}
	if id[0] == '-' || id[len(id)-1] == '-' {
		return fmt.Errorf("project ID %q must not start or end with a hyphen", id)
	}
	return nil
}
