// Package storage provides file persistence with daily rotation and retention policies.
package storage

import (
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RetentionPolicy defines the configuration for automatic cleanup of old log files.
type RetentionPolicy struct {
	Enabled       bool          // Enable/disable retention policy (default: false)
	MaxAge        int           // Delete files older than N days
	CheckInterval time.Duration // How often to check for old files
	CompressAge   int           // Compress files older than N days (0 = disabled)
}

// RetentionWorker periodically cleans up old log files based on retention policy.
type RetentionWorker struct {
	policy      RetentionPolicy
	directories []string // Directories to monitor (output dir, dlq dir, etc.)
}

// NewRetentionWorker creates a new retention worker for the specified directories.
func NewRetentionWorker(policy RetentionPolicy, directories ...string) *RetentionWorker {
	return &RetentionWorker{
		policy:      policy,
		directories: directories,
	}
}

// Start begins the retention worker in a goroutine.
// It runs cleanup immediately on start, then periodically based on CheckInterval.
// The worker stops when the context is cancelled.
func (w *RetentionWorker) Start(ctx context.Context) {
	if !w.policy.Enabled {
		slog.Info("retention policy disabled")
		return
	}

	slog.Info("starting retention worker",
		"max_age_days", w.policy.MaxAge,
		"compress_age_days", w.policy.CompressAge,
		"check_interval", w.policy.CheckInterval)

	// Run cleanup immediately on start
	w.cleanup()

	ticker := time.NewTicker(w.policy.CheckInterval)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ticker.C:
				w.cleanup()
			case <-ctx.Done():
				slog.Info("retention worker stopped")
				return
			}
		}
	}()
}

// cleanup scans all monitored directories and cleans up old files.
func (w *RetentionWorker) cleanup() {
	slog.Debug("starting retention cleanup")

	deleteCutoff := time.Now().AddDate(0, 0, -w.policy.MaxAge)
	var compressCutoff time.Time
	if w.policy.CompressAge > 0 {
		compressCutoff = time.Now().AddDate(0, 0, -w.policy.CompressAge)
	}

	totalDeleted := 0
	totalCompressed := 0
	var totalBytesFreed int64

	for _, dir := range w.directories {
		deleted, compressed, bytesFreed := w.cleanupDirectory(dir, deleteCutoff, compressCutoff)
		totalDeleted += deleted
		totalCompressed += compressed
		totalBytesFreed += bytesFreed
	}

	slog.Info("retention cleanup complete",
		"files_deleted", totalDeleted,
		"files_compressed", totalCompressed,
		"bytes_freed", totalBytesFreed)
}

// cleanupDirectory processes files in a single directory.
func (w *RetentionWorker) cleanupDirectory(dir string, deleteCutoff, compressCutoff time.Time) (deleted, compressed int, bytesFreed int64) {
	// Find all NDJSON files (both .ndjson and .ndjson.gz)
	// Matches patterns: zpa-*.ndjson, dlq-*.ndjson, and their .gz variants
	patterns := []string{
		filepath.Join(dir, "*-????-??-??.ndjson"),
		filepath.Join(dir, "*-????-??-??.ndjson.gz"),
	}

	for _, pattern := range patterns {
		files, err := filepath.Glob(pattern)
		if err != nil {
			slog.Error("failed to list files",
				"directory", dir,
				"pattern", pattern,
				"error", err)
			continue
		}

		for _, file := range files {
			fileDate := w.extractDateFromFilename(file)
			if fileDate.IsZero() {
				slog.Warn("failed to parse date from filename",
					"file", file)
				continue
			}

			// Check if file should be deleted
			if fileDate.Before(deleteCutoff) {
				size, err := w.deleteFile(file)
				if err != nil {
					slog.Error("failed to delete old file",
						"file", file,
						"error", err)
				} else {
					deleted++
					bytesFreed += size
					slog.Info("deleted old log file",
						"file", filepath.Base(file),
						"age_days", int(time.Since(fileDate).Hours()/24),
						"size_bytes", size)
				}
				continue
			}

			// Check if file should be compressed
			if w.policy.CompressAge > 0 &&
				fileDate.Before(compressCutoff) &&
				!strings.HasSuffix(file, ".gz") {
				origSize, compressedSize, err := w.compressFile(file)
				if err != nil {
					slog.Error("failed to compress file",
						"file", file,
						"error", err)
				} else {
					compressed++
					bytesFreed += (origSize - compressedSize)
					slog.Info("compressed old log file",
						"file", filepath.Base(file),
						"age_days", int(time.Since(fileDate).Hours()/24),
						"original_size", origSize,
						"compressed_size", compressedSize,
						"savings_percent", int(float64(origSize-compressedSize)/float64(origSize)*100))
				}
			}
		}
	}

	return deleted, compressed, bytesFreed
}

// extractDateFromFilename extracts the date from filenames like:
// zpa-2025-01-15.ndjson, dlq-2025-01-15.ndjson, zpa-2025-01-15.ndjson.gz
func (w *RetentionWorker) extractDateFromFilename(path string) time.Time {
	base := filepath.Base(path)

	// Remove .gz extension if present
	base = strings.TrimSuffix(base, ".gz")
	// Remove .ndjson extension
	base = strings.TrimSuffix(base, ".ndjson")

	// Find the date pattern YYYY-MM-DD at the end
	// Handle prefixes like "zpa-" or "dlq-"
	parts := strings.Split(base, "-")
	if len(parts) < 3 {
		return time.Time{}
	}

	// Get last 3 parts (YYYY, MM, DD)
	dateStr := strings.Join(parts[len(parts)-3:], "-")

	fileDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}
	}

	return fileDate
}

// deleteFile removes the file and returns its size.
func (w *RetentionWorker) deleteFile(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("failed to stat file: %w", err)
	}

	size := info.Size()

	// #nosec G304 -- path is constructed from configured directories and validated date patterns
	if err := os.Remove(path); err != nil {
		return 0, fmt.Errorf("failed to remove file: %w", err)
	}

	return size, nil
}

// compressFile compresses the file with gzip and returns the original and compressed sizes.
// The original file is deleted after successful compression.
func (w *RetentionWorker) compressFile(path string) (origSize, compressedSize int64, err error) {
	// Read original file
	// #nosec G304 -- path is constructed from configured directories and validated date patterns
	input, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read file: %w", err)
	}

	origSize = int64(len(input))

	// Create gzip file
	outputPath := path + ".gz"
	// #nosec G304 -- path is constructed from configured directories and validated date patterns
	output, err := os.Create(outputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create gzip file: %w", err)
	}
	defer output.Close()

	// Compress
	gzWriter := gzip.NewWriter(output)
	if _, err := gzWriter.Write(input); err != nil {
		_ = gzWriter.Close() // Best effort cleanup
		_ = os.Remove(outputPath)
		return 0, 0, fmt.Errorf("failed to write compressed data: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		_ = os.Remove(outputPath) // Best effort cleanup
		return 0, 0, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Get compressed file size
	info, err := os.Stat(outputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat compressed file: %w", err)
	}
	compressedSize = info.Size()

	// Delete original file after successful compression
	if err := os.Remove(path); err != nil {
		// Compression succeeded but cleanup failed - log warning but don't fail
		slog.Warn("compressed file but failed to delete original",
			"file", path,
			"compressed", outputPath,
			"error", err)
	}

	return origSize, compressedSize, nil
}
