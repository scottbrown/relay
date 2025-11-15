package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	cfg := Config{Enabled: false}
	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed with disabled config: %v", err)
	}

	if logger.Enabled() {
		t.Error("logger should not be enabled")
	}

	// Should be no-op
	err = logger.Log(Event{EventType: EventConnectionAccepted})
	if err != nil {
		t.Errorf("Log should be no-op when disabled: %v", err)
	}
}

func TestNew_Enabled(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "json",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}
	defer logger.Close()

	if !logger.Enabled() {
		t.Error("logger should be enabled")
	}
}

func TestLogger_Log_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "json",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}
	defer logger.Close()

	// Log an event
	event := Event{
		EventType:    EventConnectionAccepted,
		Success:      true,
		Actor:        "10.0.1.5",
		Action:       "connect",
		Result:       "accepted",
		ConnectionID: "test-conn-123",
	}

	err = logger.Log(event)
	if err != nil {
		t.Fatalf("Log should succeed: %v", err)
	}

	// Read the log file
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Parse JSON
	var logged Event
	err = json.Unmarshal(content[:len(content)-1], &logged) // Remove trailing newline
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify fields
	if logged.EventType != event.EventType {
		t.Errorf("expected event type %s, got %s", event.EventType, logged.EventType)
	}
	if logged.Success != event.Success {
		t.Errorf("expected success %v, got %v", event.Success, logged.Success)
	}
	if logged.Actor != event.Actor {
		t.Errorf("expected actor %s, got %s", event.Actor, logged.Actor)
	}
	if logged.Action != event.Action {
		t.Errorf("expected action %s, got %s", event.Action, logged.Action)
	}
	if logged.Result != event.Result {
		t.Errorf("expected result %s, got %s", event.Result, logged.Result)
	}
	if logged.ConnectionID != event.ConnectionID {
		t.Errorf("expected connection ID %s, got %s", event.ConnectionID, logged.ConnectionID)
	}

	// Verify timestamp was set
	if logged.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestLogger_Log_CEF(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "cef",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}
	defer logger.Close()

	// Log an event
	event := Event{
		EventType:    EventConnectionRejected,
		Success:      false,
		Actor:        "192.168.1.100",
		Action:       "connect",
		Result:       "rejected_by_acl",
		ConnectionID: "test-conn-456",
		Details: map[string]interface{}{
			"reason": "client IP not in allowed CIDR ranges",
		},
	}

	err = logger.Log(event)
	if err != nil {
		t.Fatalf("Log should succeed: %v", err)
	}

	// Read the log file
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	line := strings.TrimSpace(string(content))

	// Verify CEF format
	if !strings.HasPrefix(line, "CEF:0|Relay|ZPA Log Relay|") {
		t.Errorf("CEF log should start with correct header, got: %s", line)
	}

	// Verify key fields are present
	if !strings.Contains(line, "act=connect") {
		t.Error("CEF log should contain action")
	}
	if !strings.Contains(line, "src=192.168.1.100") {
		t.Error("CEF log should contain source IP")
	}
	if !strings.Contains(line, "outcome=rejected_by_acl") {
		t.Error("CEF log should contain outcome")
	}
}

func TestLogger_MultipleEvents(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "json",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}
	defer logger.Close()

	// Log multiple events
	events := []Event{
		{EventType: EventConnectionAccepted, Success: true, Actor: "10.0.1.1", Action: "connect", Result: "accepted"},
		{EventType: EventConnectionRejected, Success: false, Actor: "10.0.1.2", Action: "connect", Result: "rejected"},
		{EventType: EventConnectionClosed, Success: true, Actor: "10.0.1.1", Action: "disconnect", Result: "closed"},
	}

	for _, event := range events {
		err = logger.Log(event)
		if err != nil {
			t.Fatalf("Log should succeed: %v", err)
		}
	}

	// Read the log file
	file, err := os.Open(logFile)
	if err != nil {
		t.Fatalf("failed to open log file: %v", err)
	}
	defer file.Close()

	// Count lines
	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
		// Verify each line is valid JSON
		var logged Event
		err = json.Unmarshal(scanner.Bytes(), &logged)
		if err != nil {
			t.Fatalf("line %d is not valid JSON: %v", lineCount, err)
		}
	}

	if lineCount != len(events) {
		t.Errorf("expected %d lines, got %d", len(events), lineCount)
	}
}

func TestLogger_Close(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "json",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}

	// Close logger
	err = logger.Close()
	if err != nil {
		t.Errorf("Close should succeed: %v", err)
	}

	// Double close should be safe
	err = logger.Close()
	if err != nil {
		t.Errorf("double Close should be safe: %v", err)
	}

	// Log after close should be no-op
	err = logger.Log(Event{EventType: EventConnectionAccepted})
	if err != nil {
		t.Errorf("Log after close should be no-op: %v", err)
	}
}

func TestLogger_FilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "json",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}
	defer logger.Close()

	// Log an event to create the file
	err = logger.Log(Event{EventType: EventServerStart})
	if err != nil {
		t.Fatalf("Log should succeed: %v", err)
	}

	// Check file permissions
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("failed to stat log file: %v", err)
	}

	mode := info.Mode().Perm()
	expected := os.FileMode(0600)
	if mode != expected {
		t.Errorf("expected file permissions %o, got %o", expected, mode)
	}
}

func TestEventTypes(t *testing.T) {
	// Verify all event types are defined correctly
	eventTypes := []EventType{
		EventConnectionAccepted,
		EventConnectionRejected,
		EventConnectionClosed,
		EventAuthSuccess,
		EventAuthFailure,
		EventDataReceived,
		EventDataStored,
		EventDataForwarded,
		EventConfigChange,
		EventServerStart,
		EventServerStop,
	}

	for _, et := range eventTypes {
		if string(et) == "" {
			t.Errorf("event type should not be empty")
		}
	}
}

func TestEvent_Timestamp(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "json",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}
	defer logger.Close()

	// Create event without timestamp
	event := Event{
		EventType: EventConnectionAccepted,
		Success:   true,
		Actor:     "10.0.1.5",
		Action:    "connect",
		Result:    "accepted",
	}

	// Verify timestamp is zero before logging
	if !event.Timestamp.IsZero() {
		t.Error("timestamp should be zero before logging")
	}

	before := time.Now()
	err = logger.Log(event)
	if err != nil {
		t.Fatalf("Log should succeed: %v", err)
	}
	after := time.Now()

	// Read back and verify timestamp was set
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var logged Event
	err = json.Unmarshal(content[:len(content)-1], &logged)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if logged.Timestamp.IsZero() {
		t.Error("logged timestamp should not be zero")
	}

	// Verify timestamp is within reasonable range
	if logged.Timestamp.Before(before) || logged.Timestamp.After(after) {
		t.Errorf("timestamp %v should be between %v and %v", logged.Timestamp, before, after)
	}

	// Verify timestamp is UTC
	if logged.Timestamp.Location() != time.UTC {
		t.Error("timestamp should be in UTC")
	}
}

func TestLogger_Details(t *testing.T) {
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "audit.log")

	cfg := Config{
		Enabled: true,
		LogFile: logFile,
		Format:  "json",
	}

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New should succeed: %v", err)
	}
	defer logger.Close()

	// Log event with details
	event := Event{
		EventType: EventAuthFailure,
		Success:   false,
		Actor:     "10.0.1.5",
		Action:    "tls_handshake",
		Result:    "failed",
		Details: map[string]interface{}{
			"error":       "certificate expired",
			"cert_serial": "123456",
			"duration_ms": 250,
		},
	}

	err = logger.Log(event)
	if err != nil {
		t.Fatalf("Log should succeed: %v", err)
	}

	// Read and verify details
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var logged Event
	err = json.Unmarshal(content[:len(content)-1], &logged)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(logged.Details) != 3 {
		t.Errorf("expected 3 details, got %d", len(logged.Details))
	}

	if logged.Details["error"] != "certificate expired" {
		t.Errorf("expected error detail, got %v", logged.Details["error"])
	}

	if logged.Details["cert_serial"] != "123456" {
		t.Errorf("expected cert_serial detail, got %v", logged.Details["cert_serial"])
	}

	// Verify numeric detail
	durationMS, ok := logged.Details["duration_ms"].(float64)
	if !ok {
		t.Errorf("expected duration_ms to be numeric, got %T", logged.Details["duration_ms"])
	}
	if durationMS != 250 {
		t.Errorf("expected duration_ms 250, got %v", durationMS)
	}
}
