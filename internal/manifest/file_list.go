package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ship8t/ship8t/internal/strictjson"
)

// fileBody is the JSON form of a File. Size is a pointer, so a missing or null
// size is told apart from an explicit zero.
type fileBody struct {
	Path   string
	Size   *int64
	SHA256 string
}

// decodeFileBody reads one file element, accepting only the exact field names
// path, size, and sha256.
func decodeFileBody(data []byte) (fileBody, error) {
	var entry fileBody
	fields, err := strictjson.DecodeObject(data, "path", "size", "sha256")
	if err != nil {
		return entry, err
	}
	if err := strictjson.UnmarshalField(fields, "path", &entry.Path); err != nil {
		return entry, err
	}
	if err := strictjson.UnmarshalField(fields, "size", &entry.Size); err != nil {
		return entry, err
	}
	if err := strictjson.UnmarshalField(fields, "sha256", &entry.SHA256); err != nil {
		return entry, err
	}
	return entry, nil
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
	decoder := json.NewDecoder(bytes.NewReader(data))

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
			return errTooManyFiles(list.maxFiles)
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		entry, err := decodeFileBody(raw)
		if err != nil {
			return fmt.Errorf("files[%d]: %w", len(list.files), err)
		}
		if entry.Size == nil {
			return fmt.Errorf("files[%d]: size is required, use 0 for an empty file", len(list.files))
		}
		list.files = append(list.files, File{Path: entry.Path, Size: *entry.Size, SHA256: entry.SHA256})
	}
	// More also reports false at the end of the input, so only Token notices a
	// body that ends before the closing bracket.
	_, err = decoder.Token()
	return err
}
