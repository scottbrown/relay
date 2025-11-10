// Package fixtures provides utilities for loading test fixture data.
package fixtures

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// LoadFixture loads a fixture file and returns its content as a slice of lines.
// The name parameter should be the filename without the path (e.g., "valid-user-activity.ndjson").
// Each line represents one NDJSON record.
func LoadFixture(t *testing.T, name string) []string {
	t.Helper()

	content := LoadFixtureBytes(t, name)
	lines := strings.Split(string(content), "\n")

	// Remove trailing empty line if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines
}

// LoadFixtureBytes loads a fixture file and returns its raw content as bytes.
// The name parameter should be the filename without the path (e.g., "valid-user-activity.ndjson").
func LoadFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()

	path := FixturePath(name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to load fixture %s: %v", name, err)
	}

	return content
}

// FixturePath returns the absolute path to a fixture file.
// The name parameter should be the filename without the path (e.g., "valid-user-activity.ndjson").
func FixturePath(name string) string {
	// Find the project root by looking for go.mod
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			panic("Could not find project root (go.mod not found)")
		}
		dir = parent
	}

	return filepath.Join(dir, "spec", "fixtures", name)
}
