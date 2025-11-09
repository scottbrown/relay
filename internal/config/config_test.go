package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_NoFile(t *testing.T) {
	_, err := LoadConfig("")
	if err == nil {
		t.Fatal("expected error when no config file specified")
	}

	expected := "configuration file is required"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("nonexistent.yml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}

	expected := "configuration file not found"
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to contain %q, got %q", expected, err.Error())
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test.yml")

	content := `splunk:
  hec_url: "https://test.splunk.com"
  hec_token: "test-token"
  gzip: true

health_check_enabled: true
health_check_addr: ":9099"

listeners:
  - name: "user-activity"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-user-activity"
    allowed_cidrs: "10.0.0.0/8"
    max_line_bytes: 2048
    splunk:
      source_type: "zpa:user:activity"
`

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig should succeed: %v", err)
	}

	if config.Splunk.HECURL != "https://test.splunk.com" {
		t.Errorf("expected HEC URL https://test.splunk.com, got %q", config.Splunk.HECURL)
	}

	if config.Splunk.HECToken != "test-token" {
		t.Errorf("expected token test-token, got %q", config.Splunk.HECToken)
	}

	if config.Splunk.Gzip == nil || !*config.Splunk.Gzip {
		t.Error("expected gzip to be true")
	}

	if !config.HealthCheckEnabled {
		t.Error("expected health check to be enabled")
	}

	if config.HealthCheckAddr != ":9099" {
		t.Errorf("expected health check addr :9099, got %q", config.HealthCheckAddr)
	}

	if len(config.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(config.Listeners))
	}

	listener := config.Listeners[0]
	if listener.Name != "user-activity" {
		t.Errorf("expected name user-activity, got %q", listener.Name)
	}

	if listener.ListenAddr != ":9015" {
		t.Errorf("expected listen addr :9015, got %q", listener.ListenAddr)
	}

	if listener.LogType != "user-activity" {
		t.Errorf("expected log type user-activity, got %q", listener.LogType)
	}

	if listener.OutputDir != "/tmp/logs" {
		t.Errorf("expected output dir /tmp/logs, got %q", listener.OutputDir)
	}

	if listener.FilePrefix != "zpa-user-activity" {
		t.Errorf("expected file prefix zpa-user-activity, got %q", listener.FilePrefix)
	}

	if listener.AllowedCIDRs != "10.0.0.0/8" {
		t.Errorf("expected CIDRs 10.0.0.0/8, got %q", listener.AllowedCIDRs)
	}

	if listener.MaxLineBytes != 2048 {
		t.Errorf("expected max line bytes 2048, got %d", listener.MaxLineBytes)
	}

	if listener.Splunk.SourceType != "zpa:user:activity" {
		t.Errorf("expected source type zpa:user:activity, got %q", listener.Splunk.SourceType)
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
	if !strings.Contains(err.Error(), expected) {
		t.Errorf("expected error to contain %q, got %q", expected, err.Error())
	}
}

func TestLoadConfig_NoListeners(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "no-listeners.yml")

	content := `splunk:
  hec_url: "https://test.splunk.com"
  hec_token: "test-token"
`

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Fatal("expected error for no listeners")
	}

	expected := "at least one listener is required"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestLoadConfig_InvalidLogType(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "invalid-logtype.yml")

	content := `listeners:
  - name: "test"
    listen_addr: ":9015"
    log_type: "invalid-type"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-test"
    splunk:
      source_type: "zpa:test"
`

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Fatal("expected error for invalid log type")
	}

	if !strings.Contains(err.Error(), "invalid log_type") {
		t.Errorf("expected error about invalid log_type, got %q", err.Error())
	}
}

func TestLoadConfig_DuplicateListenAddr(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "duplicate-addr.yml")

	content := `listeners:
  - name: "listener1"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-user-activity"
    splunk:
      source_type: "zpa:user:activity"
  - name: "listener2"
    listen_addr: ":9015"
    log_type: "user-status"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-user-status"
    splunk:
      source_type: "zpa:user:status"
`

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	_, err := LoadConfig(configFile)
	if err == nil {
		t.Fatal("expected error for duplicate listen address")
	}

	if !strings.Contains(err.Error(), "duplicate listen_addr") {
		t.Errorf("expected error about duplicate listen_addr, got %q", err.Error())
	}
}

func TestLoadConfig_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "missing name",
			content: `listeners:
  - listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-user-activity"
`,
			expected: "name is required",
		},
		{
			name: "missing listen_addr",
			content: `listeners:
  - name: "test"
    log_type: "user-activity"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-user-activity"
`,
			expected: "listen_addr is required",
		},
		{
			name: "missing log_type",
			content: `listeners:
  - name: "test"
    listen_addr: ":9015"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-user-activity"
`,
			expected: "log_type is required",
		},
		{
			name: "missing output_dir",
			content: `listeners:
  - name: "test"
    listen_addr: ":9015"
    log_type: "user-activity"
    file_prefix: "zpa-user-activity"
`,
			expected: "output_dir is required",
		},
		{
			name: "missing file_prefix",
			content: `listeners:
  - name: "test"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/tmp/logs"
`,
			expected: "file_prefix is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configFile := filepath.Join(tmpDir, "test.yml")

			if err := os.WriteFile(configFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create config file: %v", err)
			}

			_, err := LoadConfig(configFile)
			if err == nil {
				t.Fatal("expected error")
			}

			if !strings.Contains(err.Error(), tt.expected) {
				t.Errorf("expected error to contain %q, got %q", tt.expected, err.Error())
			}
		})
	}
}

func TestLoadConfig_DefaultMaxLineBytes(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "defaults.yml")

	content := `listeners:
  - name: "test"
    listen_addr: ":9015"
    log_type: "user-activity"
    output_dir: "/tmp/logs"
    file_prefix: "zpa-user-activity"
    splunk:
      source_type: "zpa:user:activity"
`

	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("LoadConfig should succeed: %v", err)
	}

	if config.Listeners[0].MaxLineBytes != DefaultMaxLineBytes {
		t.Errorf("expected default max line bytes %d, got %d", DefaultMaxLineBytes, config.Listeners[0].MaxLineBytes)
	}

	if config.HealthCheckAddr != DefaultHealthCheckAddr {
		t.Errorf("expected default health check addr %q, got %q", DefaultHealthCheckAddr, config.HealthCheckAddr)
	}
}

func TestGetTemplate(t *testing.T) {
	template := GetTemplate()
	if template == "" {
		t.Error("template should not be empty")
	}

	expectedStrings := []string{
		"listeners:",
		"name:",
		"listen_addr:",
		"log_type:",
		"output_dir:",
		"file_prefix:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(template, expected) {
			t.Errorf("template should contain %q", expected)
		}
	}
}

func TestConstants(t *testing.T) {
	if DefaultMaxLineBytes != 1<<20 {
		t.Errorf("expected default max line bytes %d, got %d", 1<<20, DefaultMaxLineBytes)
	}

	if DefaultHealthCheckAddr != ":9099" {
		t.Errorf("expected default health check addr :9099, got %q", DefaultHealthCheckAddr)
	}

	if DefaultHealthCheckEnabled != false {
		t.Errorf("expected default health check enabled false, got %v", DefaultHealthCheckEnabled)
	}
}
