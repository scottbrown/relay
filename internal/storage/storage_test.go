package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew_Success(t *testing.T) {
	tmpDir := t.TempDir()

	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}

	if manager.baseDir != tmpDir {
		t.Errorf("expected base dir %q, got %q", tmpDir, manager.baseDir)
	}

	if manager.filePrefix != "zpa" {
		t.Errorf("expected file prefix %q, got %q", "zpa", manager.filePrefix)
	}

	if manager.file != nil {
		t.Error("file should be nil initially")
	}

	if manager.curDay != "" {
		t.Error("curDay should be empty initially")
	}
}

func TestNew_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "directory")

	manager, err := New(nestedDir, "zpa")
	if err != nil {
		t.Fatalf("New should create nested directories: %v", err)
	}

	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("directory should exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("should be a directory")
	}

	if manager.baseDir != nestedDir {
		t.Errorf("expected base dir %q, got %q", nestedDir, manager.baseDir)
	}
}

func TestNew_InvalidDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "existing-file")

	// Create a file first
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Try to create manager with file path as directory
	_, err := New(existingFile, "zpa")
	if err == nil {
		t.Fatal("expected error when trying to use file as directory")
	}
}

func TestWrite_FirstWrite(t *testing.T) {
	tmpDir := t.TempDir()
	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	testData := []byte(`{"test": "data"}`)
	err = manager.Write(testData)
	if err != nil {
		t.Fatalf("Write should succeed: %v", err)
	}

	if manager.file == nil {
		t.Error("file should be open after first write")
	}

	if manager.curDay == "" {
		t.Error("curDay should be set after first write")
	}

	expectedDay := time.Now().UTC().Format("2006-01-02")
	if manager.curDay != expectedDay {
		t.Errorf("expected curDay %q, got %q", expectedDay, manager.curDay)
	}

	// Verify file exists and contains data
	expectedPath := filepath.Join(tmpDir, "zpa-"+expectedDay+".ndjson")
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expectedContent := string(testData) + "\n"
	if string(content) != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, string(content))
	}
}

func TestWrite_MultipleWrites(t *testing.T) {
	tmpDir := t.TempDir()
	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	testData1 := []byte(`{"test": "data1"}`)
	testData2 := []byte(`{"test": "data2"}`)

	err = manager.Write(testData1)
	if err != nil {
		t.Fatalf("first Write should succeed: %v", err)
	}

	err = manager.Write(testData2)
	if err != nil {
		t.Fatalf("second Write should succeed: %v", err)
	}

	// Verify file contains both lines
	expectedDay := time.Now().UTC().Format("2006-01-02")
	expectedPath := filepath.Join(tmpDir, "zpa-"+expectedDay+".ndjson")
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expectedContent := string(testData1) + "\n" + string(testData2) + "\n"
	if string(content) != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, string(content))
	}
}

func TestWrite_DayRotation(t *testing.T) {
	tmpDir := t.TempDir()
	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	// Write initial data
	testData1 := []byte(`{"test": "data1"}`)
	err = manager.Write(testData1)
	if err != nil {
		t.Fatalf("first Write should succeed: %v", err)
	}

	originalFile := manager.file

	// Simulate day change by manually setting curDay to yesterday
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	manager.curDay = yesterday

	// Write more data - should trigger rotation
	testData2 := []byte(`{"test": "data2"}`)
	err = manager.Write(testData2)
	if err != nil {
		t.Fatalf("Write after rotation should succeed: %v", err)
	}

	// Verify rotation occurred
	if manager.curDay == yesterday {
		t.Error("curDay should have been updated")
	}

	if manager.file == originalFile {
		t.Error("file should have been rotated")
	}

	// Verify new file was created
	expectedDay := time.Now().UTC().Format("2006-01-02")
	if manager.curDay != expectedDay {
		t.Errorf("expected curDay %q, got %q", expectedDay, manager.curDay)
	}
}

func TestCurrentFile(t *testing.T) {
	tmpDir := t.TempDir()
	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	// Initially should return empty string
	if path := manager.CurrentFile(); path != "" {
		t.Errorf("expected empty path initially, got %q", path)
	}

	// After first write, should return current file path
	testData := []byte(`{"test": "data"}`)
	err = manager.Write(testData)
	if err != nil {
		t.Fatalf("Write should succeed: %v", err)
	}

	currentPath := manager.CurrentFile()
	expectedDay := time.Now().UTC().Format("2006-01-02")
	expectedPath := filepath.Join(tmpDir, "zpa-"+expectedDay+".ndjson")

	if currentPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, currentPath)
	}
}

func TestClose_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	err = manager.Close()
	if err != nil {
		t.Errorf("Close with no file should succeed: %v", err)
	}
}

func TestClose_WithFile(t *testing.T) {
	tmpDir := t.TempDir()
	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Write to open a file
	testData := []byte(`{"test": "data"}`)
	err = manager.Write(testData)
	if err != nil {
		t.Fatalf("Write should succeed: %v", err)
	}

	if manager.file == nil {
		t.Fatal("file should be open")
	}

	err = manager.Close()
	if err != nil {
		t.Errorf("Close should succeed: %v", err)
	}
}

func TestEnsureDir_Success(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test", "nested", "directory")

	err := ensureDir(testDir)
	if err != nil {
		t.Fatalf("ensureDir should succeed: %v", err)
	}

	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("directory should exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("should be a directory")
	}
}

func TestEnsureDir_ExistingDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	err := ensureDir(tmpDir)
	if err != nil {
		t.Errorf("ensureDir with existing directory should succeed: %v", err)
	}
}

func TestOpenDayFile(t *testing.T) {
	tmpDir := t.TempDir()
	manager, err := New(tmpDir, "zpa")
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	day := "2023-01-15"
	file, err := manager.openDayFile(day)
	if err != nil {
		t.Fatalf("openDayFile should succeed: %v", err)
	}
	defer file.Close()

	expectedPath := filepath.Join(tmpDir, "zpa-"+day+".ndjson")
	info, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}

	if info.IsDir() {
		t.Error("should be a file, not directory")
	}

	// Test writing to file
	testData := []byte("test data\n")
	_, err = file.Write(testData)
	if err != nil {
		t.Fatalf("should be able to write to file: %v", err)
	}
}

func TestCustomFilePrefix(t *testing.T) {
	tmpDir := t.TempDir()
	customPrefix := "zpa-user-activity"
	manager, err := New(tmpDir, customPrefix)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	defer manager.Close()

	// Write test data
	testData := []byte(`{"test": "data"}`)
	err = manager.Write(testData)
	if err != nil {
		t.Fatalf("Write should succeed: %v", err)
	}

	// Verify file was created with custom prefix
	expectedDay := time.Now().UTC().Format("2006-01-02")
	expectedPath := filepath.Join(tmpDir, customPrefix+"-"+expectedDay+".ndjson")
	content, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expectedContent := string(testData) + "\n"
	if string(content) != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, string(content))
	}

	// Verify CurrentFile returns correct path
	currentPath := manager.CurrentFile()
	if currentPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, currentPath)
	}
}
