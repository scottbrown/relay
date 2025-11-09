package storage

import (
	"os"
	"path/filepath"
	"time"
)

// Manager handles file persistence with daily rotation
type Manager struct {
	baseDir    string
	filePrefix string
	file       *os.File
	curDay     string
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
func (m *Manager) Write(data []byte) error {
	day := time.Now().UTC().Format("2006-01-02")

	if day != m.curDay {
		if m.file != nil {
			m.file.Close()
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
	return filepath.Join(m.baseDir, m.filePrefix+"-"+m.curDay+".ndjson")
}

func (m *Manager) openDayFile(day string) (*os.File, error) {
	path := filepath.Join(m.baseDir, m.filePrefix+"-"+day+".ndjson")
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}
