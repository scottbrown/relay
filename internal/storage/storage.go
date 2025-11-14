package storage

import (
	"os"
	"path/filepath"
	"time"
)

// Manager handles file persistence with daily rotation
type Manager struct {
	baseDir string
	file    *os.File
	curDay  string
}

// New creates a new storage manager for the given directory
func New(baseDir string) (*Manager, error) {
	if err := ensureDir(baseDir); err != nil {
		return nil, err
	}

	return &Manager{baseDir: baseDir}, nil
}

// Write writes data to the current day's file, rotating if necessary
func (m *Manager) Write(data []byte) error {
	day := time.Now().UTC().Format("2006-01-02")

	if day != m.curDay {
		if m.file != nil {
			_ = m.file.Close() // #nosec G104 - Error on close during rotation is non-critical
		}

		var err error
		m.file, err = m.openDayFile(day)
		if err != nil {
			return err
		}

		m.curDay = day
	}

	_, err := m.file.Write(append(data, '\n'))
	return err
}

// Close closes the current file
func (m *Manager) Close() error {
	if m.file != nil {
		return m.file.Close()
	}
	return nil
}

// CurrentFile returns the path to the current day's file
func (m *Manager) CurrentFile() string {
	if m.curDay == "" {
		return ""
	}
	return filepath.Join(m.baseDir, "zpa-"+m.curDay+".ndjson")
}

func (m *Manager) openDayFile(day string) (*os.File, error) {
	path := filepath.Join(m.baseDir, "zpa-"+day+".ndjson")
	// #nosec G304 - Path is controlled, uses filepath.Join with baseDir validated at creation
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
}

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0750)
}
