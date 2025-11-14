package storage

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewRetentionWorker(t *testing.T) {
	policy := RetentionPolicy{
		Enabled:       true,
		MaxAge:        30,
		CheckInterval: 1 * time.Hour,
		CompressAge:   7,
	}

	worker := NewRetentionWorker(policy, "/var/log/relay", "/var/log/relay/dlq")

	if worker == nil {
		t.Fatal("expected worker to be created")
	}
	if worker.policy != policy {
		t.Errorf("policy mismatch: got %+v, want %+v", worker.policy, policy)
	}
	if len(worker.directories) != 2 {
		t.Errorf("expected 2 directories, got %d", len(worker.directories))
	}
}

func TestRetentionWorker_DeleteOldFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files with different ages
	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	todayDate := time.Now().Format("2006-01-02")

	oldFile := filepath.Join(tmpDir, "zpa-"+oldDate+".ndjson")
	recentFile := filepath.Join(tmpDir, "zpa-"+recentDate+".ndjson")
	todayFile := filepath.Join(tmpDir, "zpa-"+todayDate+".ndjson")

	createTestFile(t, oldFile, "old data\n")
	createTestFile(t, recentFile, "recent data\n")
	createTestFile(t, todayFile, "today data\n")

	policy := RetentionPolicy{
		Enabled:       true,
		MaxAge:        30, // Delete files older than 30 days
		CheckInterval: 1 * time.Hour,
	}

	worker := NewRetentionWorker(policy, tmpDir)
	worker.cleanup()

	// Old file should be deleted
	if fileExists(oldFile) {
		t.Errorf("expected old file to be deleted: %s", oldFile)
	}

	// Recent and today files should still exist
	if !fileExists(recentFile) {
		t.Errorf("expected recent file to exist: %s", recentFile)
	}
	if !fileExists(todayFile) {
		t.Errorf("expected today file to exist: %s", todayFile)
	}
}

func TestRetentionWorker_CompressOldFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files with different ages
	oldDate := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	recentDate := time.Now().AddDate(0, 0, -3).Format("2006-01-02")

	oldFile := filepath.Join(tmpDir, "zpa-"+oldDate+".ndjson")
	recentFile := filepath.Join(tmpDir, "zpa-"+recentDate+".ndjson")

	testData := "test data that should be compressed\n"
	createTestFile(t, oldFile, testData)
	createTestFile(t, recentFile, "recent data\n")

	policy := RetentionPolicy{
		Enabled:       true,
		MaxAge:        30,
		CheckInterval: 1 * time.Hour,
		CompressAge:   7, // Compress files older than 7 days
	}

	worker := NewRetentionWorker(policy, tmpDir)
	worker.cleanup()

	// Old file should be compressed and original deleted
	if fileExists(oldFile) {
		t.Errorf("expected original file to be deleted: %s", oldFile)
	}
	if !fileExists(oldFile + ".gz") {
		t.Errorf("expected compressed file to exist: %s", oldFile+".gz")
	}

	// Recent file should not be compressed
	if !fileExists(recentFile) {
		t.Errorf("expected recent file to exist: %s", recentFile)
	}
	if fileExists(recentFile + ".gz") {
		t.Errorf("expected recent file not to be compressed: %s", recentFile+".gz")
	}

	// Verify compressed data is valid
	data, err := readGzipFile(oldFile + ".gz")
	if err != nil {
		t.Fatalf("failed to read compressed file: %v", err)
	}
	if string(data) != testData {
		t.Errorf("compressed data mismatch: got %q, want %q", string(data), testData)
	}
}

func TestRetentionWorker_DLQFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create DLQ files
	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	recentDate := time.Now().AddDate(0, 0, -5).Format("2006-01-02")

	oldFile := filepath.Join(tmpDir, "dlq-"+oldDate+".ndjson")
	recentFile := filepath.Join(tmpDir, "dlq-"+recentDate+".ndjson")

	createTestFile(t, oldFile, "old dlq data\n")
	createTestFile(t, recentFile, "recent dlq data\n")

	policy := RetentionPolicy{
		Enabled:       true,
		MaxAge:        30,
		CheckInterval: 1 * time.Hour,
	}

	worker := NewRetentionWorker(policy, tmpDir)
	worker.cleanup()

	// Old DLQ file should be deleted
	if fileExists(oldFile) {
		t.Errorf("expected old DLQ file to be deleted: %s", oldFile)
	}

	// Recent DLQ file should still exist
	if !fileExists(recentFile) {
		t.Errorf("expected recent DLQ file to exist: %s", recentFile)
	}
}

func TestRetentionWorker_CompressedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create already compressed file that's old enough to delete
	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	oldFile := filepath.Join(tmpDir, "zpa-"+oldDate+".ndjson.gz")

	// Create gzip file
	f, err := os.Create(oldFile)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	gzWriter := gzip.NewWriter(f)
	if _, err := gzWriter.Write([]byte("compressed data\n")); err != nil {
		t.Fatalf("failed to write compressed data: %v", err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close file: %v", err)
	}

	policy := RetentionPolicy{
		Enabled:       true,
		MaxAge:        30,
		CheckInterval: 1 * time.Hour,
	}

	worker := NewRetentionWorker(policy, tmpDir)
	worker.cleanup()

	// Old compressed file should be deleted
	if fileExists(oldFile) {
		t.Errorf("expected old compressed file to be deleted: %s", oldFile)
	}
}

func TestRetentionWorker_MultipleDirectories(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")

	oldFile1 := filepath.Join(tmpDir1, "zpa-"+oldDate+".ndjson")
	oldFile2 := filepath.Join(tmpDir2, "dlq-"+oldDate+".ndjson")

	createTestFile(t, oldFile1, "old data 1\n")
	createTestFile(t, oldFile2, "old data 2\n")

	policy := RetentionPolicy{
		Enabled:       true,
		MaxAge:        30,
		CheckInterval: 1 * time.Hour,
	}

	worker := NewRetentionWorker(policy, tmpDir1, tmpDir2)
	worker.cleanup()

	// Both old files should be deleted
	if fileExists(oldFile1) {
		t.Errorf("expected old file in dir1 to be deleted: %s", oldFile1)
	}
	if fileExists(oldFile2) {
		t.Errorf("expected old file in dir2 to be deleted: %s", oldFile2)
	}
}

func TestRetentionWorker_DisabledPolicy(t *testing.T) {
	tmpDir := t.TempDir()

	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	oldFile := filepath.Join(tmpDir, "zpa-"+oldDate+".ndjson")

	createTestFile(t, oldFile, "old data\n")

	policy := RetentionPolicy{
		Enabled:       false, // Disabled
		MaxAge:        30,
		CheckInterval: 1 * time.Hour,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := NewRetentionWorker(policy, tmpDir)
	worker.Start(ctx)

	// File should not be deleted when policy is disabled
	if !fileExists(oldFile) {
		t.Errorf("expected file to still exist when policy is disabled: %s", oldFile)
	}
}

func TestRetentionWorker_Start(t *testing.T) {
	tmpDir := t.TempDir()

	oldDate := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	oldFile := filepath.Join(tmpDir, "zpa-"+oldDate+".ndjson")

	createTestFile(t, oldFile, "old data\n")

	policy := RetentionPolicy{
		Enabled:       true,
		MaxAge:        30,
		CheckInterval: 100 * time.Millisecond, // Short interval for testing
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	worker := NewRetentionWorker(policy, tmpDir)
	worker.Start(ctx)

	// Give the worker a moment to run
	time.Sleep(200 * time.Millisecond)

	// File should be deleted by the worker
	if fileExists(oldFile) {
		t.Errorf("expected file to be deleted by worker: %s", oldFile)
	}

	// Cancel context to stop worker
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestExtractDateFromFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string // Expected date in YYYY-MM-DD format, or empty for error
	}{
		{
			name:     "standard log file",
			filename: "zpa-2025-01-15.ndjson",
			want:     "2025-01-15",
		},
		{
			name:     "DLQ file",
			filename: "dlq-2025-01-15.ndjson",
			want:     "2025-01-15",
		},
		{
			name:     "compressed log file",
			filename: "zpa-2025-01-15.ndjson.gz",
			want:     "2025-01-15",
		},
		{
			name:     "compressed DLQ file",
			filename: "dlq-2025-01-15.ndjson.gz",
			want:     "2025-01-15",
		},
		{
			name:     "with full path",
			filename: "/var/log/relay/zpa-2025-01-15.ndjson",
			want:     "2025-01-15",
		},
		{
			name:     "custom prefix",
			filename: "custom-prefix-2025-01-15.ndjson",
			want:     "2025-01-15",
		},
		{
			name:     "invalid filename",
			filename: "invalid.ndjson",
			want:     "",
		},
		{
			name:     "wrong date format",
			filename: "zpa-01-15-2025.ndjson",
			want:     "",
		},
	}

	worker := NewRetentionWorker(RetentionPolicy{}, ".")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := worker.extractDateFromFilename(tt.filename)

			if tt.want == "" {
				if !got.IsZero() {
					t.Errorf("expected zero time for invalid filename, got %v", got)
				}
			} else {
				if got.Format("2006-01-02") != tt.want {
					t.Errorf("date mismatch: got %s, want %s", got.Format("2006-01-02"), tt.want)
				}
			}
		})
	}
}

func TestCompressFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ndjson")
	// Use larger test data to ensure compression actually reduces size
	testData := strings.Repeat("test data for compression line\n", 100)

	createTestFile(t, testFile, testData)

	worker := NewRetentionWorker(RetentionPolicy{}, tmpDir)
	origSize, compressedSize, err := worker.compressFile(testFile)

	if err != nil {
		t.Fatalf("failed to compress file: %v", err)
	}
	if origSize != int64(len(testData)) {
		t.Errorf("original size mismatch: got %d, want %d", origSize, len(testData))
	}
	if compressedSize <= 0 {
		t.Errorf("expected positive compressed size, got %d", compressedSize)
	}
	// For larger repetitive data, compression should reduce size
	if compressedSize >= origSize {
		t.Errorf("expected compressed size (%d) to be less than original (%d)", compressedSize, origSize)
	}

	// Original file should be deleted
	if fileExists(testFile) {
		t.Errorf("expected original file to be deleted: %s", testFile)
	}

	// Compressed file should exist
	compressedFile := testFile + ".gz"
	if !fileExists(compressedFile) {
		t.Errorf("expected compressed file to exist: %s", compressedFile)
	}

	// Verify compressed data
	data, err := readGzipFile(compressedFile)
	if err != nil {
		t.Fatalf("failed to read compressed file: %v", err)
	}
	if string(data) != testData {
		t.Errorf("compressed data mismatch: got %q, want %q", string(data), testData)
	}
}

func TestDeleteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.ndjson")
	testData := "test data\n"

	createTestFile(t, testFile, testData)

	worker := NewRetentionWorker(RetentionPolicy{}, tmpDir)
	size, err := worker.deleteFile(testFile)

	if err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}
	if size != int64(len(testData)) {
		t.Errorf("size mismatch: got %d, want %d", size, len(testData))
	}

	// File should be deleted
	if fileExists(testFile) {
		t.Errorf("expected file to be deleted: %s", testFile)
	}
}

func TestDeleteFile_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nonexistent.ndjson")

	worker := NewRetentionWorker(RetentionPolicy{}, tmpDir)
	_, err := worker.deleteFile(testFile)

	if err == nil {
		t.Error("expected error when deleting non-existent file")
	}
}

// Helper functions

func createTestFile(t *testing.T, path string, content string) {
	t.Helper()
	err := os.WriteFile(path, []byte(content), 0600)
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readGzipFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	return io.ReadAll(gzReader)
}
