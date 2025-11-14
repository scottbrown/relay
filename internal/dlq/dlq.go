// Package dlq implements a dead letter queue for failed HEC forwards.
// Failed messages are written to NDJSON files with metadata for later analysis or replay.
package dlq

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/scottbrown/relay/internal/metrics"
)

// Entry represents a failed forward entry in the dead letter queue.
// It contains the original data plus metadata about the failure.
type Entry struct {
	Timestamp string `json:"timestamp"` // ISO 8601 timestamp of failure
	ConnID    string `json:"conn_id"`   // Connection ID for correlation
	Error     string `json:"error"`     // Error message describing the failure
	Data      string `json:"data"`      // Original log line that failed to forward
}

// Writer handles writing failed forwards to the dead letter queue.
// Files are rotated daily and named: dlq-YYYY-MM-DD.ndjson.
//
// Writer is safe for concurrent use by multiple goroutines.
type Writer struct {
	baseDir string
	file    *os.File
	curDay  string
	mu      sync.Mutex
}

// New creates a new DLQ Writer for the given directory.
// The directory is created if it does not exist.
// Returns an error if the directory cannot be created.
func New(baseDir string) (*Writer, error) {
	if err := ensureDir(baseDir); err != nil {
		return nil, err
	}

	return &Writer{
		baseDir: baseDir,
	}, nil
}

// Write writes a failed forward entry to the DLQ with metadata.
// The entry includes timestamp, connection ID, error message, and original data.
func (w *Writer) Write(connID string, data []byte, err error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	day := time.Now().UTC().Format("2006-01-02")

	if day != w.curDay {
		if w.file != nil {
			if closeErr := w.file.Close(); closeErr != nil {
				return closeErr
			}
		}

		var openErr error
		w.file, openErr = w.openDayFile(day)
		if openErr != nil {
			return openErr
		}
		w.curDay = day
	}

	// Create DLQ entry with metadata
	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		ConnID:    connID,
		Error:     err.Error(),
		Data:      string(data),
	}

	// Marshal to JSON
	jsonData, marshalErr := json.Marshal(entry)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal DLQ entry: %w", marshalErr)
	}

	// Write with newline
	if _, writeErr := w.file.Write(append(jsonData, '\n')); writeErr != nil {
		return writeErr
	}

	metrics.LinesProcessed.Add("dlq", 1)
	slog.Debug("wrote to DLQ", "conn_id", connID, "error", err.Error())
	return nil
}

// Close closes the current day's file if open.
// Returns an error if the file cannot be closed.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// openDayFile opens or creates the DLQ file for the given day.
// Files are named: dlq-YYYY-MM-DD.ndjson
func (w *Writer) openDayFile(day string) (*os.File, error) {
	filename := filepath.Join(w.baseDir, fmt.Sprintf("dlq-%s.ndjson", day))
	// #nosec G304 -- baseDir is set during Writer construction from config.
	// The day parameter is generated from time.Now() and used for daily DLQ rotation.
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	slog.Info("opened DLQ file", "path", filename)
	return file, nil
}

// ensureDir creates the directory if it doesn't exist.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return nil
}

// CurrentFile returns the path to the current day's DLQ file.
// Returns empty string if no file is currently open.
func (w *Writer) CurrentFile() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return ""
	}
	return w.file.Name()
}
