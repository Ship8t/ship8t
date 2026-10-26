package manifest

import "errors"

// checksumLength is the length of a SHA-256 checksum in hexadecimal characters.
const checksumLength = 64

// validate checks the path, the size, and the checksum of one file.
func (file File) validate() error {
	if err := ValidatePath(file.Path); err != nil {
		return err
	}
	if file.Size < 0 {
		return errors.New("invalid size: size must not be negative")
	}
	if !isLowercaseSHA256(file.SHA256) {
		return errors.New("invalid sha256: want 64 lowercase hexadecimal characters")
	}
	return nil
}

// isLowercaseSHA256 reports whether checksum is 64 lowercase hexadecimal
// characters. It checks bytes, so a multibyte character never passes.
func isLowercaseSHA256(checksum string) bool {
	if len(checksum) != checksumLength {
		return false
	}
	for index := 0; index < len(checksum); index++ {
		character := checksum[index]
		isDigit := character >= '0' && character <= '9'
		isLowercaseLetter := character >= 'a' && character <= 'f'
		if !isDigit && !isLowercaseLetter {
			return false
		}
	}
	return true
}
