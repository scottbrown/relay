package relay

import (
	"strings"
	"testing"
)

func TestVersion_DefaultValues(t *testing.T) {
	// When version and build are not set (default empty strings)
	version = ""
	build = ""

	result := Version()
	expected := " ()"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVersion_WithVersionOnly(t *testing.T) {
	version = "1.0.0"
	build = ""
	defer func() {
		version = ""
		build = ""
	}()

	result := Version()
	expected := "1.0.0 ()"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVersion_WithBuildOnly(t *testing.T) {
	version = ""
	build = "abc123"
	defer func() {
		version = ""
		build = ""
	}()

	result := Version()
	expected := " (abc123)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVersion_WithBoth(t *testing.T) {
	version = "1.2.3"
	build = "def456"
	defer func() {
		version = ""
		build = ""
	}()

	result := Version()
	expected := "1.2.3 (def456)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVersion_Format(t *testing.T) {
	version = "v1.0.0"
	build = "commit-hash-123"
	defer func() {
		version = ""
		build = ""
	}()

	result := Version()

	// Verify the format is consistent
	if !strings.Contains(result, version) {
		t.Errorf("result should contain version %q, got %q", version, result)
	}

	if !strings.Contains(result, build) {
		t.Errorf("result should contain build %q, got %q", build, result)
	}

	if !strings.Contains(result, " (") {
		t.Errorf("result should contain ' (', got %q", result)
	}

	if !strings.Contains(result, ")") {
		t.Errorf("result should contain ')', got %q", result)
	}
}

func TestVersion_Injection(t *testing.T) {
	// Test that variables can be set (simulating ldflags injection)
	originalVersion := version
	originalBuild := build
	defer func() {
		version = originalVersion
		build = originalBuild
	}()

	testVersion := "test-version"
	testBuild := "test-build"

	version = testVersion
	build = testBuild

	result := Version()
	expected := testVersion + " (" + testBuild + ")"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVersion_RealWorldExample(t *testing.T) {
	version = "v1.5.2"
	build = "2023-12-01T10:30:00Z-abc1234"
	defer func() {
		version = ""
		build = ""
	}()

	result := Version()
	expected := "v1.5.2 (2023-12-01T10:30:00Z-abc1234)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVersion_SpecialCharacters(t *testing.T) {
	version = "1.0.0-beta.1"
	build = "feature/test-123_abc"
	defer func() {
		version = ""
		build = ""
	}()

	result := Version()
	expected := "1.0.0-beta.1 (feature/test-123_abc)"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
