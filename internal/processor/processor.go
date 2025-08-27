package processor

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ReadLineLimited reads a line from the reader with a maximum byte limit
func ReadLineLimited(br *bufio.Reader, limit int) ([]byte, error) {
	var buf bytes.Buffer

	for {
		b, err := br.ReadBytes('\n')
		buf.Write(b)

		if len(buf.Bytes()) > limit {
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

// IsValidJSON checks if the given byte slice contains valid JSON
func IsValidJSON(data []byte) bool {
	return json.Valid(data)
}

// Truncate truncates a byte slice to a maximum length, adding ellipsis if truncated
func Truncate(data []byte, maxLen int) string {
	s := string(data)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}
