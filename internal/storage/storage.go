package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager handles file persistence with daily rotation
type Manager struct {
	baseDir    string
	filePrefix string
	file       *os.File
	curDay     string
	mu         sync.Mutex
}

// New creates a new storage manager for the given directory with the specified file prefix
func New(baseDir, filePrefix string) (*Manager, error) {
	if err := ensureDir(baseDir); err != nil {
		return nil, err
	}

	return &Manager{
		baseDir:    baseDir,
		filePrefix: filePrefix,
	}, nil
}

// Write writes data to the current day's file, rotating if necessary
func (m *Manager) Write(connID string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	day := time.Now().UTC().Format("2006-01-02")

	if day != m.curDay {
		if m.file != nil {
			if err := m.file.Close(); err != nil {
				return err
			}
		}

		var err error
		m.file, err = m.openDayFile(day)
		if err != nil {
			return err
		}

		m.curDay = day
	}

	n, err := m.file.Write(append(data, '\n'))
	if err == nil {
		slog.Debug("stored line", "conn_id", connID, "bytes", n)
	}
	return err
}

// Close closes the current file
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.file != nil {
		// Sync to ensure all data is flushed to disk
		if err := m.file.Sync(); err != nil {
			// Try to close the file even if sync failed
			_ = m.file.Close()
			return err
		}
		return m.file.Close()
	}
	return nil
}

// CurrentFile returns the path to the current day's file
func (m *Manager) CurrentFile() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.curDay == "" {
		return ""
	}
	return filepath.Join(m.baseDir, m.filePrefix+"-"+m.curDay+".ndjson")
}

func (m *Manager) openDayFile(day string) (*os.File, error) {
	path := filepath.Join(m.baseDir, m.filePrefix+"-"+day+".ndjson")
	// #nosec G304 -- baseDir and filePrefix are set during Manager construction from config.
	// The day parameter is generated from time.Now() and used for daily log rotation.
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
}

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0700)
}
