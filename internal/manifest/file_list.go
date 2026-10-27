package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// fileBody is the JSON form of a File. Size is a pointer, so a missing or null
// size is told apart from an explicit zero.
type fileBody struct {
	Path   string `json:"path"`
	Size   *int64 `json:"size"`
	SHA256 string `json:"sha256"`
}

// fileList decodes the files array of a manifest and stops once the array
// holds more than maxFiles entries, so a small body cannot expand into a huge
// slice before the file count limit runs.
type fileList struct {
	maxFiles int
	files    []File
}

// UnmarshalJSON reads the array one file at a time and fails with
// ErrLimitExceeded before it decodes a file past maxFiles.
func (list *fileList) UnmarshalJSON(data []byte) error {
	// Replace anything a repeated files key decoded before.
	list.files = nil

	// Decode again with a new decoder, because the outer decoder's strict
	// field setting does not reach into UnmarshalJSON.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("files must be a list of files, not null")
	}
	if token != json.Delim('[') {
		return errors.New("files must be a list of files")
	}
	for decoder.More() {
		if len(list.files) >= list.maxFiles {
			return fmt.Errorf("%w: more than the limit of %d files per deployment", ErrLimitExceeded, list.maxFiles)
		}
		var entry fileBody
		if err := decoder.Decode(&entry); err != nil {
			return err
		}
		if entry.Size == nil {
			return fmt.Errorf("files[%d]: size is required, use 0 for an empty file", len(list.files))
		}
		list.files = append(list.files, File{Path: entry.Path, Size: *entry.Size, SHA256: entry.SHA256})
	}
	// Read the closing bracket.
	_, err = decoder.Token()
	return err
}
