// Package processor provides utilities for processing incoming log data.
// It includes functions for reading size-limited lines, validating JSON, and truncating data.
package processor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ReadLineLimited reads a line from the reader with a maximum byte limit.
// If a line exceeds the limit, it drains the remaining data until a newline is found
// and returns an error, ensuring the reader is positioned correctly for the next line.
// Returns the line without trailing newline characters on success.
func ReadLineLimited(br *bufio.Reader, limit int) ([]byte, error) {
	var buf bytes.Buffer

	for {
		b, err := br.ReadBytes('\n')
		buf.Write(b)

		if len(buf.Bytes()) > limit {
			// Line exceeds limit - drain the rest until we find a newline
			// to ensure the next read starts at the correct position
			if err == nil || !errors.Is(err, io.EOF) {
				// We have a complete chunk but haven't hit newline yet, keep draining
				for !bytes.Contains(b, []byte{'\n'}) {
					b, err = br.ReadBytes('\n')
					if err != nil {
						break
					}
				}
			}
			return nil, errors.New("line exceeds limit")
		}

		if err != nil {
			if errors.Is(err, io.EOF) && buf.Len() > 0 {
				return bytes.TrimRight(buf.Bytes(), "\r\n"), nil
			}
			return nil, err
		}

		// Got newline
		return bytes.TrimRight(buf.Bytes(), "\r\n"), nil
	}
}

// IsValidJSON checks if the given byte slice contains valid JSON.
// It uses the standard library's json.Valid function for validation.
func IsValidJSON(data []byte) bool {
	return json.Valid(data)
}

// Truncate truncates a byte slice to a maximum length, adding ellipsis if truncated.
// This is useful for logging long lines without overwhelming log output.
func Truncate(data []byte, maxLen int) string {
	s := string(data)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
