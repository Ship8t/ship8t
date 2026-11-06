package manifest

import "fmt"

// checkChecksumSizes reports a file whose checksum matches an earlier file but
// whose size differs, because a checksum names one content and so one size. It
// compares each file with the first file that has the same checksum. It expects
// checksums that already passed validation.
func checkChecksumSizes(files []File) error {
	firstByChecksum := make(map[string]int, len(files))
	for index, file := range files {
		first, found := firstByChecksum[file.SHA256]
		if !found {
			firstByChecksum[file.SHA256] = index
			continue
		}
		if files[first].Size != file.Size {
			return fmt.Errorf("files[%d] has size %d but files[%d] has the same sha256 and size %d", index, file.Size, first, files[first].Size)
		}
	}
	return nil
}
