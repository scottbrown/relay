// Package relaytest provides utilities for launching and managing Relay instances in tests.
package relaytest

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// HECConfig represents Splunk HEC configuration for testing.
type HECConfig struct {
	URL        string
	Token      string
	Sourcetype string
	UseGzip    bool
}

// TLSConfig represents TLS configuration for testing.
type TLSConfig struct {
	CertFile string
	KeyFile  string
}

// RelayInstance represents a running Relay service instance for testing.
type RelayInstance struct {
	// Configuration
	ListenAddr   string
	StorageDir   string
	FilePrefix   string
	LogType      string
	HECConfig    *HECConfig
	TLSConfig    *TLSConfig
	MaxLineBytes int
	AllowedCIDRs string

	// Runtime
	cmd        *exec.Cmd
	stdout     *bytes.Buffer
	stderr     *bytes.Buffer
	configFile string
	t          *testing.T
}

// Option is a functional option for configuring RelayInstance.
type Option func(*RelayInstance)

// WithHEC configures the Relay instance to forward to the specified HEC endpoint.
func WithHEC(url, token, sourcetype string, useGzip bool) Option {
	return func(r *RelayInstance) {
		r.HECConfig = &HECConfig{
			URL:        url,
			Token:      token,
			Sourcetype: sourcetype,
			UseGzip:    useGzip,
		}
	}
}

// WithTLS configures the Relay instance to use TLS for incoming connections.
func WithTLS(certFile, keyFile string) Option {
	return func(r *RelayInstance) {
		r.TLSConfig = &TLSConfig{
			CertFile: certFile,
			KeyFile:  keyFile,
		}
	}
}

// WithMaxLineBytes sets the maximum line size.
func WithMaxLineBytes(maxBytes int) Option {
	return func(r *RelayInstance) {
		r.MaxLineBytes = maxBytes
	}
}

// WithAllowedCIDRs sets the allowed CIDR list for access control.
func WithAllowedCIDRs(cidrs string) Option {
	return func(r *RelayInstance) {
		r.AllowedCIDRs = cidrs
	}
}

// WithListenAddr sets a specific listen address instead of auto-allocating.
func WithListenAddr(addr string) Option {
	return func(r *RelayInstance) {
		r.ListenAddr = addr
	}
}

// WithLogType sets the log type for the listener.
func WithLogType(logType string) Option {
	return func(r *RelayInstance) {
		r.LogType = logType
	}
}

// NewRelayInstance creates a new Relay instance for testing.
func NewRelayInstance(t *testing.T, opts ...Option) *RelayInstance {
	t.Helper()

	// Use temp directory for storage
	storageDir := t.TempDir()

	// Auto-allocate a port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to allocate port: %v", err)
	}
	listenAddr := listener.Addr().String()
	listener.Close()

	instance := &RelayInstance{
		ListenAddr:   listenAddr,
		StorageDir:   storageDir,
		FilePrefix:   "test",
		LogType:      "user-activity",
		MaxLineBytes: 1 << 20, // 1 MiB default
		t:            t,
	}

	// Apply options
	for _, opt := range opts {
		opt(instance)
	}

	return instance
}

// Start starts the Relay service.
func (r *RelayInstance) Start() error {
	r.t.Helper()

	// Generate config file
	configFile, err := r.generateConfigFile()
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}
	r.configFile = configFile

	// Build the relay binary if needed
	if err := r.buildRelay(); err != nil {
		return fmt.Errorf("failed to build relay: %w", err)
	}

	// Find project root
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	binaryPath := filepath.Join(projectRoot, ".build", "relay")

	// Prepare command
	r.cmd = exec.Command(binaryPath, "--config", configFile)

	r.stdout = &bytes.Buffer{}
	r.stderr = &bytes.Buffer{}
	r.cmd.Stdout = r.stdout
	r.cmd.Stderr = r.stderr

	// Start the process
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start relay: %w", err)
	}

	return nil
}

// WaitForReady waits for the Relay service to be ready to accept connections.
func (r *RelayInstance) WaitForReady(timeout time.Duration) error {
	r.t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to connect
		conn, err := net.DialTimeout("tcp", r.ListenAddr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}

		// Check if process has exited
		if r.cmd.ProcessState != nil && r.cmd.ProcessState.Exited() {
			return fmt.Errorf("relay process exited prematurely: stdout=%s stderr=%s",
				r.stdout.String(), r.stderr.String())
		}

		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for relay to be ready: stdout=%s stderr=%s",
		r.stdout.String(), r.stderr.String())
}

// Stop stops the Relay service.
func (r *RelayInstance) Stop() error {
	r.t.Helper()

	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}

	// Send interrupt signal
	if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
		// If interrupt fails, try kill
		r.cmd.Process.Kill()
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- r.cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		r.cmd.Process.Kill()
		return fmt.Errorf("timeout waiting for relay to stop, killed")
	}
}

// Logs returns the stdout and stderr output from the Relay service.
func (r *RelayInstance) Logs() (stdout, stderr string) {
	r.t.Helper()

	if r.stdout != nil {
		stdout = r.stdout.String()
	}
	if r.stderr != nil {
		stderr = r.stderr.String()
	}

	return stdout, stderr
}

// StorageFiles returns a list of files in the storage directory.
func (r *RelayInstance) StorageFiles() ([]string, error) {
	r.t.Helper()

	entries, err := os.ReadDir(r.StorageDir)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// ReadStorageFile reads the contents of a storage file and returns it as lines.
func (r *RelayInstance) ReadStorageFile(filename string) ([]string, error) {
	r.t.Helper()

	path := filepath.Join(r.StorageDir, filename)
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(content), "\n")
	// Remove trailing empty line if present
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines, nil
}

// generateConfigFile generates a temporary YAML config file for the Relay instance.
func (r *RelayInstance) generateConfigFile() (string, error) {
	r.t.Helper()

	// Build config structure
	config := map[string]interface{}{
		"health_check_enabled": false,
		"listeners": []map[string]interface{}{
			{
				"name":           "test",
				"listen_addr":    r.ListenAddr,
				"log_type":       r.LogType,
				"output_dir":     r.StorageDir,
				"file_prefix":    r.FilePrefix,
				"max_line_bytes": r.MaxLineBytes,
			},
		},
	}

	listener := config["listeners"].([]map[string]interface{})[0]

	// Add TLS if configured
	if r.TLSConfig != nil {
		listener["tls"] = map[string]string{
			"cert_file": r.TLSConfig.CertFile,
			"key_file":  r.TLSConfig.KeyFile,
		}
	}

	// Add ACL if configured
	if r.AllowedCIDRs != "" {
		listener["allowed_cidrs"] = r.AllowedCIDRs
	}

	// Add HEC config if configured
	if r.HECConfig != nil {
		listener["splunk"] = map[string]interface{}{
			"hec_url":     r.HECConfig.URL,
			"hec_token":   r.HECConfig.Token,
			"source_type": r.HECConfig.Sourcetype,
			"gzip":        r.HECConfig.UseGzip,
		}
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to temp file
	configFile := filepath.Join(r.t.TempDir(), "relay-config.yml")
	if err := os.WriteFile(configFile, yamlBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to write config file: %w", err)
	}

	return configFile, nil
}

// buildRelay builds the relay binary if it doesn't exist.
func (r *RelayInstance) buildRelay() error {
	r.t.Helper()

	projectRoot, err := findProjectRoot()
	if err != nil {
		return err
	}

	binaryPath := filepath.Join(projectRoot, ".build", "relay")

	// Check if binary exists and is recent
	if stat, err := os.Stat(binaryPath); err == nil {
		// If binary was built in the last 5 minutes, use it
		if time.Since(stat.ModTime()) < 5*time.Minute {
			return nil
		}
	}

	// Build using task
	cmd := exec.Command("task", "build")
	cmd.Dir = projectRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("task build failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// findProjectRoot finds the project root by looking for go.mod.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find project root (go.mod not found)")
		}
		dir = parent
	}
}

// MustStart starts the Relay instance and waits for it to be ready, or fails the test.
func (r *RelayInstance) MustStart(ctx context.Context) {
	r.t.Helper()

	if err := r.Start(); err != nil {
		r.t.Fatalf("Failed to start relay: %v", err)
	}

	if err := r.WaitForReady(10 * time.Second); err != nil {
		stdout, stderr := r.Logs()
		r.t.Fatalf("Relay not ready: %v\nStdout:\n%s\nStderr:\n%s", err, stdout, stderr)
	}
}
