package processor

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadLineLimited_Success(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		limit    int
		expected string
	}{
		{
			name:     "simple line with newline",
			input:    "hello world\n",
			limit:    100,
			expected: "hello world",
		},
		{
			name:     "line with CRLF",
			input:    "hello world\r\n",
			limit:    100,
			expected: "hello world",
		},
		{
			name:     "line without newline",
			input:    "hello world",
			limit:    100,
			expected: "hello world",
		},
		{
			name:     "empty line",
			input:    "\n",
			limit:    100,
			expected: "",
		},
		{
			name:     "line at limit",
			input:    "12345\n",
			limit:    6, // includes newline
			expected: "12345",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := bufio.NewReader(strings.NewReader(tt.input))
			result, err := ReadLineLimited(br, tt.limit)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, string(result))
			}
		})
	}
}

func TestReadLineLimited_ExceedsLimit(t *testing.T) {
	input := "this line is too long for the limit\n"
	limit := 10

	br := bufio.NewReader(strings.NewReader(input))
	_, err := ReadLineLimited(br, limit)
	if err == nil {
		t.Fatal("expected error for line exceeding limit")
	}

	expectedMsg := "line exceeds limit"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
	}
}

func TestReadLineLimited_EOF(t *testing.T) {
	input := ""
	br := bufio.NewReader(strings.NewReader(input))
	_, err := ReadLineLimited(br, 100)
	if err != io.EOF {
		t.Errorf("expected EOF error, got %v", err)
	}
}

func TestReadLineLimited_EOFWithContent(t *testing.T) {
	input := "no newline at end"
	br := bufio.NewReader(strings.NewReader(input))
	result, err := ReadLineLimited(br, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != input {
		t.Errorf("expected %q, got %q", input, string(result))
	}
}

func TestReadLineLimited_ReadError(t *testing.T) {
	expectedErr := errors.New("read error")
	errorReader := &errorReader{err: expectedErr}
	br := bufio.NewReader(errorReader)

	_, err := ReadLineLimited(br, 100)
	if err != expectedErr {
		t.Errorf("expected specific read error, got %v", err)
	}
}

func TestReadLineLimited_MultipleLines(t *testing.T) {
	input := "first line\nsecond line\nthird line\n"
	br := bufio.NewReader(strings.NewReader(input))

	expectedLines := []string{"first line", "second line", "third line"}

	for i, expected := range expectedLines {
		result, err := ReadLineLimited(br, 100)
		if err != nil {
			t.Fatalf("line %d: unexpected error: %v", i+1, err)
		}
		if string(result) != expected {
			t.Errorf("line %d: expected %q, got %q", i+1, expected, string(result))
		}
	}

	// Should get EOF on next read
	_, err := ReadLineLimited(br, 100)
	if err != io.EOF {
		t.Errorf("expected EOF after all lines read, got %v", err)
	}
}

func TestIsValidJSON_Valid(t *testing.T) {
	validJSON := [][]byte{
		[]byte(`{}`),
		[]byte(`{"key": "value"}`),
		[]byte(`{"nested": {"key": "value"}}`),
		[]byte(`[]`),
		[]byte(`[1, 2, 3]`),
		[]byte(`[{"key": "value"}]`),
		[]byte(`null`),
		[]byte(`true`),
		[]byte(`false`),
		[]byte(`123`),
		[]byte(`"string"`),
		[]byte(`{"number": 123, "string": "test", "bool": true, "null": null}`),
	}

	for i, json := range validJSON {
		if !IsValidJSON(json) {
			t.Errorf("test %d: expected valid JSON for %q", i+1, string(json))
		}
	}
}

func TestIsValidJSON_Invalid(t *testing.T) {
	invalidJSON := [][]byte{
		[]byte(`{`),
		[]byte(`}`),
		[]byte(`{"key": }`),
		[]byte(`{"key": "value",}`),
		[]byte(`[1, 2,]`),
		[]byte(`invalid`),
		[]byte(`{key: "value"}`),
		[]byte(`{'key': 'value'}`),
		[]byte(`undefined`),
		[]byte(``),
	}

	for i, json := range invalidJSON {
		if IsValidJSON(json) {
			t.Errorf("test %d: expected invalid JSON for %q", i+1, string(json))
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		maxLen   int
		expected string
	}{
		{
			name:     "no truncation needed",
			input:    []byte("hello"),
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length",
			input:    []byte("hello"),
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "needs truncation",
			input:    []byte("hello world"),
			maxLen:   5,
			expected: "hello…",
		},
		{
			name:     "empty input",
			input:    []byte(""),
			maxLen:   10,
			expected: "",
		},
		{
			name:     "zero max length",
			input:    []byte("hello"),
			maxLen:   0,
			expected: "…",
		},
		{
			name:     "unicode input",
			input:    []byte("hello world"),
			maxLen:   7,
			expected: "hello w…",
		},
		{
			name:     "very long input",
			input:    bytes.Repeat([]byte("a"), 1000),
			maxLen:   10,
			expected: "aaaaaaaaaa…",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// errorReader is a helper that always returns an error when read
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}
