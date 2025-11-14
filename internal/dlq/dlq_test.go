package dlq

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_Success(t *testing.T) {
	dir := t.TempDir()

	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if writer == nil {
		t.Fatal("New() returned nil writer")
	}

	if writer.baseDir != dir {
		t.Errorf("baseDir = %v, want %v", writer.baseDir, dir)
	}
}

func TestNew_CreateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "subdir", "dlq")

	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Check directory was created
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("directory was not created")
	}

	if writer.baseDir != dir {
		t.Errorf("baseDir = %v, want %v", writer.baseDir, dir)
	}
}

func TestWrite_FirstWrite(t *testing.T) {
	dir := t.TempDir()
	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer writer.Close()

	testErr := errors.New("hec send failed")
	testData := []byte(`{"test":"data"}`)
	testConnID := "test-conn-123"

	if err := writer.Write(testConnID, testData, testErr); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Check file was created
	currentFile := writer.CurrentFile()
	if currentFile == "" {
		t.Fatal("no current file after write")
	}

	// Read and verify content
	content, err := os.ReadFile(currentFile)
	if err != nil {
		t.Fatalf("failed to read DLQ file: %v", err)
	}

	// Parse JSON
	var entry Entry
	if err := json.Unmarshal(content[:len(content)-1], &entry); err != nil { // trim newline
		t.Fatalf("failed to parse DLQ entry: %v", err)
	}

	// Verify entry fields
	if entry.ConnID != testConnID {
		t.Errorf("entry.ConnID = %v, want %v", entry.ConnID, testConnID)
	}
	if entry.Error != testErr.Error() {
		t.Errorf("entry.Error = %v, want %v", entry.Error, testErr.Error())
	}
	if entry.Data != string(testData) {
		t.Errorf("entry.Data = %v, want %v", entry.Data, string(testData))
	}
	if entry.Timestamp == "" {
		t.Error("entry.Timestamp is empty")
	}

	// Verify timestamp is valid ISO 8601
	if _, err := time.Parse(time.RFC3339, entry.Timestamp); err != nil {
		t.Errorf("invalid timestamp format: %v", err)
	}
}

func TestWrite_MultipleWrites(t *testing.T) {
	dir := t.TempDir()
	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer writer.Close()

	// Write multiple entries
	for i := 0; i < 3; i++ {
		testErr := errors.New("test error")
		testData := []byte(`{"line":"` + string(rune('A'+i)) + `"}`)
		testConnID := "conn-" + string(rune('1'+i))

		if err := writer.Write(testConnID, testData, testErr); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	// Read and verify content
	content, err := os.ReadFile(writer.CurrentFile())
	if err != nil {
		t.Fatalf("failed to read DLQ file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}

	// Verify each entry
	for i, line := range lines {
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("failed to parse line %d: %v", i, err)
		}

		expectedConnID := "conn-" + string(rune('1'+i))
		if entry.ConnID != expectedConnID {
			t.Errorf("line %d: conn_id = %v, want %v", i, entry.ConnID, expectedConnID)
		}
	}
}

func TestWrite_DayRotation(t *testing.T) {
	dir := t.TempDir()
	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer writer.Close()

	// Write first entry
	testErr := errors.New("test error")
	if err := writer.Write("conn-1", []byte(`{"a":"1"}`), testErr); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Verify first write created a file with today's date
	day := time.Now().UTC().Format("2006-01-02")
	expectedFilename := filepath.Join(dir, "dlq-"+day+".ndjson")

	if _, err := os.Stat(expectedFilename); os.IsNotExist(err) {
		t.Errorf("expected file %s does not exist", expectedFilename)
	}

	// Read and verify content
	content, err := os.ReadFile(expectedFilename)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line, got %d", len(lines))
	}

	// Verify entry can be parsed
	var entry Entry
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatalf("failed to parse entry: %v", err)
	}

	if entry.ConnID != "conn-1" {
		t.Errorf("entry.ConnID = %v, want conn-1", entry.ConnID)
	}
}

func TestClose_NoFile(t *testing.T) {
	dir := t.TempDir()
	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Close without writing
	if err := writer.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestClose_WithFile(t *testing.T) {
	dir := t.TempDir()
	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Write to create file
	testErr := errors.New("test error")
	if err := writer.Write("conn-1", []byte(`{"test":"data"}`), testErr); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	filePath := writer.CurrentFile()

	// Close
	if err := writer.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Verify file still exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("file was deleted after close")
	}
}

func TestCurrentFile(t *testing.T) {
	dir := t.TempDir()
	writer, err := New(dir)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer writer.Close()

	// No file initially
	if current := writer.CurrentFile(); current != "" {
		t.Errorf("CurrentFile() = %v, want empty string", current)
	}

	// Write to create file
	testErr := errors.New("test error")
	if err := writer.Write("conn-1", []byte(`{"test":"data"}`), testErr); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	// Should have current file
	current := writer.CurrentFile()
	if current == "" {
		t.Error("CurrentFile() returned empty string after write")
	}

	// Check filename format
	day := time.Now().UTC().Format("2006-01-02")
	expectedFilename := "dlq-" + day + ".ndjson"
	if !strings.HasSuffix(current, expectedFilename) {
		t.Errorf("CurrentFile() = %v, want to end with %v", current, expectedFilename)
	}
}

func TestEntry_JSONMarshaling(t *testing.T) {
	entry := Entry{
		Timestamp: "2025-11-14T15:30:00Z",
		ConnID:    "test-conn",
		Error:     "test error",
		Data:      `{"test":"data"}`,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal back
	var decoded Entry
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify fields
	if decoded.Timestamp != entry.Timestamp {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, entry.Timestamp)
	}
	if decoded.ConnID != entry.ConnID {
		t.Errorf("ConnID = %v, want %v", decoded.ConnID, entry.ConnID)
	}
	if decoded.Error != entry.Error {
		t.Errorf("Error = %v, want %v", decoded.Error, entry.Error)
	}
	if decoded.Data != entry.Data {
		t.Errorf("Data = %v, want %v", decoded.Data, entry.Data)
	}
}
