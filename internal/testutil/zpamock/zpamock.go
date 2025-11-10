// Package zpamock provides a mock ZPA client for integration testing.
package zpamock

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

// MockZPAClient simulates a ZPA App Connector streaming NDJSON logs to a relay service.
type MockZPAClient struct {
	// Configuration
	Address   string
	UseTLS    bool
	TLSConfig *tls.Config

	// Behaviour
	LineDelay time.Duration

	// State
	conn      net.Conn
	LinesSent int
	Errors    []error

	// Observability
	verbose bool
}

// Option is a functional option for configuring MockZPAClient.
type Option func(*MockZPAClient)

// WithTLS enables TLS with the provided configuration.
func WithTLS(config *tls.Config) Option {
	return func(c *MockZPAClient) {
		c.UseTLS = true
		c.TLSConfig = config
	}
}

// WithLineDelay sets the delay between sending lines.
func WithLineDelay(delay time.Duration) Option {
	return func(c *MockZPAClient) {
		c.LineDelay = delay
	}
}

// WithVerbose enables verbose logging.
func WithVerbose(verbose bool) Option {
	return func(c *MockZPAClient) {
		c.verbose = verbose
	}
}

// New creates a new MockZPAClient configured to connect to the specified address.
func New(address string, opts ...Option) *MockZPAClient {
	client := &MockZPAClient{
		Address:   address,
		LinesSent: 0,
		Errors:    make([]error, 0),
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// Connect establishes a connection to the relay service.
func (c *MockZPAClient) Connect(ctx context.Context) error {
	c.logEvent("connecting", map[string]interface{}{
		"address": c.Address,
		"tls":     c.UseTLS,
	})

	var conn net.Conn
	var err error

	dialer := &net.Dialer{}

	if c.UseTLS {
		if c.TLSConfig == nil {
			// #nosec G402 -- InsecureSkipVerify is acceptable here because this is a test utility
			// that needs to connect to relay servers with self-signed certificates. This code is
			// never used in production and is isolated to the internal/testutil package.
			c.TLSConfig = &tls.Config{
				InsecureSkipVerify: true, // For testing only
			}
		}
		conn, err = tls.DialWithDialer(dialer, "tcp", c.Address, c.TLSConfig)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", c.Address)
	}

	if err != nil {
		c.recordError(err)
		c.logEvent("connection_failed", map[string]interface{}{
			"error": err.Error(),
		})
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.conn = conn
	c.logEvent("connected", map[string]interface{}{
		"local_addr":  conn.LocalAddr().String(),
		"remote_addr": conn.RemoteAddr().String(),
	})

	return nil
}

// SendLine sends a single line (NDJSON record) to the relay.
func (c *MockZPAClient) SendLine(line string) error {
	if c.conn == nil {
		return fmt.Errorf("not connected")
	}

	// Apply line delay if configured
	if c.LineDelay > 0 {
		time.Sleep(c.LineDelay)
	}

	// Ensure line ends with newline
	if !strings.HasSuffix(line, "\n") {
		line = line + "\n"
	}

	_, err := c.conn.Write([]byte(line))
	if err != nil {
		c.recordError(err)
		c.logEvent("send_failed", map[string]interface{}{
			"error": err.Error(),
			"line":  c.LinesSent + 1,
		})
		return fmt.Errorf("failed to send line: %w", err)
	}

	c.LinesSent++
	c.logEvent("line_sent", map[string]interface{}{
		"line_number": c.LinesSent,
		"bytes":       len(line),
	})

	return nil
}

// SendLines sends multiple lines to the relay.
func (c *MockZPAClient) SendLines(lines []string) error {
	for i, line := range lines {
		if err := c.SendLine(line); err != nil {
			return fmt.Errorf("failed to send line %d: %w", i, err)
		}
	}

	c.logEvent("batch_sent", map[string]interface{}{
		"lines": len(lines),
	})

	return nil
}

// Close closes the connection to the relay.
func (c *MockZPAClient) Close() error {
	if c.conn == nil {
		return nil
	}

	c.logEvent("closing", map[string]interface{}{
		"lines_sent": c.LinesSent,
		"errors":     len(c.Errors),
	})

	err := c.conn.Close()
	c.conn = nil

	if err != nil {
		c.recordError(err)
		return err
	}

	c.logEvent("closed", nil)
	return nil
}

// recordError records an error.
func (c *MockZPAClient) recordError(err error) {
	c.Errors = append(c.Errors, err)
}

// logEvent logs structured events to stdout if verbose mode is enabled.
func (c *MockZPAClient) logEvent(event string, data map[string]interface{}) {
	if !c.verbose {
		return
	}

	logEntry := map[string]interface{}{
		"level":     "info",
		"component": "zpamock",
		"event":     event,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	for k, v := range data {
		logEntry[k] = v
	}

	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("Failed to marshal log entry: %v", err)
		return
	}

	fmt.Println(string(jsonBytes))
}

// Error Injection Helpers

// TruncatedJSON returns a truncated version of the input JSON (missing closing brace).
func TruncatedJSON(validJSON string) string {
	if len(validJSON) < 10 {
		return validJSON
	}
	// Remove last character (should be closing brace or newline)
	truncated := validJSON
	if strings.HasSuffix(truncated, "\n") {
		truncated = truncated[:len(truncated)-1]
	}
	if strings.HasSuffix(truncated, "}") {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

// OversizedLine generates a line of the specified size (in bytes).
func OversizedLine(size int) string {
	if size < 50 {
		size = 50 // Minimum size for valid JSON structure
	}

	// Create a JSON object with a large payload field
	payload := strings.Repeat("A", size-50) // Reserve 50 bytes for JSON structure
	return fmt.Sprintf(`{"LogTimestamp":"test","Payload":"%s"}`, payload)
}

// BlankLine returns an empty line.
func BlankLine() string {
	return ""
}

// InvalidUTF8 returns a string with invalid UTF-8 sequences.
func InvalidUTF8() string {
	return "{\"data\":\"invalid\xff\xfe\"}"
}

// InvalidJSON returns syntactically invalid JSON.
func InvalidJSON() string {
	return "{invalid json without quotes}"
}

// MissingClosingBrace returns JSON missing its closing brace.
func MissingClosingBrace() string {
	return `{"LogTimestamp":"test","Customer":"test"`
}
