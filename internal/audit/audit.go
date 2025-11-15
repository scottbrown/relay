package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// EventType represents the type of audit event.
type EventType string

// Audit event types for security and compliance tracking.
const (
	EventConnectionAccepted EventType = "connection.accepted"
	EventConnectionRejected EventType = "connection.rejected"
	EventConnectionClosed   EventType = "connection.closed"
	EventAuthSuccess        EventType = "auth.success"
	EventAuthFailure        EventType = "auth.failure"
	EventDataReceived       EventType = "data.received"
	EventDataStored         EventType = "data.stored"
	EventDataForwarded      EventType = "data.forwarded"
	EventConfigChange       EventType = "config.changed"
	EventServerStart        EventType = "server.start"
	EventServerStop         EventType = "server.stop"
)

// Event represents a single audit log entry with security-relevant information.
type Event struct {
	Timestamp    time.Time              `json:"timestamp"`               // Event timestamp in UTC
	EventType    EventType              `json:"event_type"`              // Type of event
	Success      bool                   `json:"success"`                 // Whether the action succeeded
	Actor        string                 `json:"actor"`                   // Client IP or certificate CN
	Resource     string                 `json:"resource,omitempty"`      // Resource being accessed
	Action       string                 `json:"action"`                  // Action being performed
	Result       string                 `json:"result"`                  // Result of the action
	Details      map[string]interface{} `json:"details,omitempty"`       // Additional event details
	ConnectionID string                 `json:"connection_id,omitempty"` // Connection correlation ID
}

// Config holds audit logging configuration.
type Config struct {
	Enabled     bool   // Enable audit logging
	LogFile     string // Path to audit log file
	Format      string // Output format: "json" or "cef"
	IncludeData bool   // Include data in audit logs (PII concern)
}

// Logger writes audit events to a dedicated audit log file.
// It ensures events are written atomically and immediately flushed to disk.
type Logger struct {
	file   *os.File
	mu     sync.Mutex
	cfg    Config
	closed bool
}

// New creates a new audit logger with the given configuration.
// Returns a no-op logger if audit logging is disabled.
// The audit log file is created with restrictive permissions (0600).
func New(cfg Config) (*Logger, error) {
	if !cfg.Enabled {
		return &Logger{cfg: cfg}, nil // No-op logger
	}

	// Create audit log file with restrictive permissions
	f, err := os.OpenFile(cfg.LogFile,
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0600) // Only owner can read/write
	if err != nil {
		return nil, err
	}

	return &Logger{
		file: f,
		cfg:  cfg,
	}, nil
}

// Log writes an audit event to the audit log.
// Events are automatically timestamped and formatted based on configuration.
// Returns nil if audit logging is disabled.
func (al *Logger) Log(event Event) error {
	if al.file == nil || al.closed {
		return nil // Audit logging disabled or closed
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	// Set timestamp to current UTC time
	event.Timestamp = time.Now().UTC()

	var line []byte
	var err error

	// Format based on configuration
	if al.cfg.Format == "cef" {
		line = formatCEF(event)
	} else {
		line, err = json.Marshal(event)
		if err != nil {
			return err
		}
	}

	// Write event to file
	if _, err := al.file.Write(append(line, '\n')); err != nil {
		return err
	}

	// Ensure written to disk immediately for audit trail integrity
	return al.file.Sync()
}

// Close closes the audit log file.
// Should be called when shutting down the application.
func (al *Logger) Close() error {
	if al.file == nil {
		return nil
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	if al.closed {
		return nil
	}

	al.closed = true
	return al.file.Close()
}

// Enabled returns whether audit logging is enabled.
func (al *Logger) Enabled() bool {
	return al.cfg.Enabled && al.file != nil
}
