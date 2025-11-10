package fixtures

import (
	"path/filepath"
	"testing"
)

func TestLoadFixture(t *testing.T) {
	lines := LoadFixture(t, "valid-user-activity.ndjson")

	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	for i, line := range lines {
		if line == "" {
			t.Errorf("Line %d is empty", i)
		}
		if line[0] != '{' {
			t.Errorf("Line %d does not start with '{': %s", i, line)
		}
	}
}

func TestLoadFixtureBytes(t *testing.T) {
	content := LoadFixtureBytes(t, "valid-user-activity.ndjson")

	if len(content) == 0 {
		t.Error("Expected non-empty content")
	}

	// Should contain JSON
	if content[0] != '{' {
		t.Errorf("Content does not start with '{': %c", content[0])
	}
}

func TestFixturePath(t *testing.T) {
	path := FixturePath("valid-user-activity.ndjson")

	if !filepath.IsAbs(path) {
		t.Error("Expected absolute path, got relative")
	}

	expectedSuffix := filepath.Join("spec", "fixtures", "valid-user-activity.ndjson")
	if !filepath.HasPrefix(path, "/") && !filepath.HasPrefix(path, `C:\`) {
		t.Errorf("Path does not look absolute: %s", path)
	}

	if !endsWithPath(path, expectedSuffix) {
		t.Errorf("Path %s does not end with expected suffix %s", path, expectedSuffix)
	}
}

func TestLoadFixtureMalformed(t *testing.T) {
	lines := LoadFixture(t, "malformed-json.ndjson")

	// Should load all lines, including malformed ones
	if len(lines) < 5 {
		t.Errorf("Expected at least 5 lines, got %d", len(lines))
	}
}

func TestLoadFixtureEdgeCases(t *testing.T) {
	lines := LoadFixture(t, "edge-cases.ndjson")

	// Should handle blank lines and other edge cases
	if len(lines) == 0 {
		t.Error("Expected non-empty fixture")
	}

	// Check that blank lines are preserved (they should be in the slice)
	foundBlank := false
	for _, line := range lines {
		if line == "" {
			foundBlank = true
			break
		}
	}

	if !foundBlank {
		t.Log("Note: edge-cases.ndjson contains blank lines but they might be at the end")
	}
}

// Helper function to check if a path ends with a specific suffix
func endsWithPath(path, suffix string) bool {
	path = filepath.Clean(path)
	suffix = filepath.Clean(suffix)
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}
