package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	config, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig with empty string should succeed: %v", err)
	}

	if config.ListenAddr != DefaultListenAddr {
		t.Errorf("expected listen addr %q, got %q", DefaultListenAddr, config.ListenAddr)
	}

	if config.OutputDir != DefaultOutputDir {
		t.Errorf("expected output dir %q, got %q", DefaultOutputDir, config.OutputDir)
	}

	if config.SourceType != DefaultSourceType {
		t.Errorf("expected source type %q, got %q", DefaultSourceType, config.SourceType)
	}

	if config.MaxLineBytes != DefaultMaxLineBytes {
		t.Errorf("expected max line bytes %d, got %d", DefaultMaxLineBytes, config.MaxLineBytes)
	}

	if !config.GzipHEC {
		t.Error("expected gzip HEC to be true by default")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent.yml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	expected := "configuration file not found"
	if err.Error()[:len(expected)] != expected {
		t.Errorf("expected error message to start with %q, got %q", expected, err.Error())
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test.yml")

	content := `listen_addr: ":8080"
output_dir: "/tmp/logs"
splunk_hec_url: "https://test.splunk.com"
splunk_token: "test-token"
source_type: "test:type"
allowed_cidrs: "10.0.0.0/8,192.168.1.0/24"
gzip_hec: false
max_line_bytes: 2048
`

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig should succeed: %v", err)
	}

	if config.ListenAddr != ":8080" {
		t.Errorf("expected listen addr :8080, got %q", config.ListenAddr)
	}

	if config.OutputDir != "/tmp/logs" {
		t.Errorf("expected output dir /tmp/logs, got %q", config.OutputDir)
	}

	if config.SplunkHECURL != "https://test.splunk.com" {
		t.Errorf("expected HEC URL https://test.splunk.com, got %q", config.SplunkHECURL)
	}

	if config.SplunkToken != "test-token" {
		t.Errorf("expected token test-token, got %q", config.SplunkToken)
	}

	if config.SourceType != "test:type" {
		t.Errorf("expected source type test:type, got %q", config.SourceType)
	}

	if config.AllowedCIDRs != "10.0.0.0/8,192.168.1.0/24" {
		t.Errorf("expected CIDRs 10.0.0.0/8,192.168.1.0/24, got %q", config.AllowedCIDRs)
	}

	if config.GzipHEC {
		t.Error("expected gzip HEC to be false")
	}

	if config.MaxLineBytes != 2048 {
		t.Errorf("expected max line bytes 2048, got %d", config.MaxLineBytes)
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "invalid.yml")

	content := `invalid: yaml: content: [unclosed
`

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create invalid config file: %v", err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	expected := "failed to parse YAML config"
	if err.Error()[:len(expected)] != expected {
		t.Errorf("expected error message to start with %q, got %q", expected, err.Error())
	}
}

func TestLoadConfig_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "unreadable.yml")

	if err := os.WriteFile(configFile, []byte("test: content"), 0000); err != nil {
		t.Fatalf("failed to create unreadable config file: %v", err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Fatal("expected error for unreadable file")
	}

	expected := "failed to read config file"
	if err.Error()[:len(expected)] != expected {
		t.Errorf("expected error message to start with %q, got %q", expected, err.Error())
	}
}

func TestGetTemplate(t *testing.T) {
	template := GetTemplate()
	if template == "" {
		t.Error("template should not be empty")
	}

	// Template should contain key configuration options
	expectedStrings := []string{
		"listen_addr:",
		"output_dir:",
		"splunk_hec_url:",
		"splunk_token:",
		"source_type:",
		"allowed_cidrs:",
		"gzip_hec:",
		"max_line_bytes:",
	}

	for _, expected := range expectedStrings {
		if !containsString(template, expected) {
			t.Errorf("template should contain %q", expected)
		}
	}
}

func TestConstants(t *testing.T) {
	if DefaultListenAddr != ":9015" {
		t.Errorf("expected default listen addr :9015, got %q", DefaultListenAddr)
	}

	if DefaultOutputDir != "./zpa-logs" {
		t.Errorf("expected default output dir ./zpa-logs, got %q", DefaultOutputDir)
	}

	if DefaultSourceType != "zpa:lss" {
		t.Errorf("expected default source type zpa:lss, got %q", DefaultSourceType)
	}

	if DefaultMaxLineBytes != 1<<20 {
		t.Errorf("expected default max line bytes %d, got %d", 1<<20, DefaultMaxLineBytes)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr ||
		(len(s) > len(substr) && containsStringRec(s[1:], substr))
}

func containsStringRec(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	if s[:len(substr)] == substr {
		return true
	}
	return containsStringRec(s[1:], substr)
}
