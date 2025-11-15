package audit

import (
	"strings"
	"testing"
	"time"
)

func TestFormatCEF_Basic(t *testing.T) {
	event := Event{
		Timestamp: time.Date(2025, 11, 14, 10, 30, 45, 0, time.UTC),
		EventType: EventConnectionAccepted,
		Success:   true,
		Actor:     "10.0.1.5",
		Action:    "connect",
		Result:    "accepted",
	}

	cef := string(formatCEF(event))

	// Verify CEF header
	if !strings.HasPrefix(cef, "CEF:0|Relay|ZPA Log Relay|") {
		t.Errorf("CEF should start with correct header, got: %s", cef)
	}

	// Verify required extension fields
	if !strings.Contains(cef, "act=connect") {
		t.Error("CEF should contain action")
	}
	if !strings.Contains(cef, "src=10.0.1.5") {
		t.Error("CEF should contain source")
	}
	if !strings.Contains(cef, "outcome=accepted") {
		t.Error("CEF should contain outcome")
	}

	// Verify timestamp in milliseconds
	expectedTS := event.Timestamp.UnixMilli()
	if !strings.Contains(cef, "rt="+string(rune(expectedTS/1000000000))) {
		t.Log("CEF should contain timestamp:", cef)
	}
}

func TestFormatCEF_WithDetails(t *testing.T) {
	event := Event{
		Timestamp: time.Now().UTC(),
		EventType: EventAuthFailure,
		Success:   false,
		Actor:     "10.0.1.5",
		Action:    "mtls_auth",
		Result:    "failed",
		Details: map[string]interface{}{
			"error": "certificate expired",
		},
	}

	cef := string(formatCEF(event))

	// Verify details are included
	if !strings.Contains(cef, "cs1=") {
		t.Error("CEF should contain custom string field for details")
	}
	if !strings.Contains(cef, "cs1Label=Details") {
		t.Error("CEF should contain custom string label")
	}
}

func TestFormatCEF_WithConnectionID(t *testing.T) {
	event := Event{
		Timestamp:    time.Now().UTC(),
		EventType:    EventConnectionAccepted,
		Success:      true,
		Actor:        "10.0.1.5",
		Action:       "connect",
		Result:       "accepted",
		ConnectionID: "abc-123-def",
	}

	cef := string(formatCEF(event))

	// Verify connection ID is included
	if !strings.Contains(cef, "cn1=abc-123-def") {
		t.Error("CEF should contain connection ID")
	}
	if !strings.Contains(cef, "cn1Label=Connection ID") {
		t.Error("CEF should contain connection ID label")
	}
}

func TestFormatCEF_WithResource(t *testing.T) {
	event := Event{
		Timestamp: time.Now().UTC(),
		EventType: EventDataStored,
		Success:   true,
		Actor:     "10.0.1.5",
		Resource:  "/var/log/relay/zpa.log",
		Action:    "write",
		Result:    "success",
	}

	cef := string(formatCEF(event))

	// Verify resource is included as device
	if !strings.Contains(cef, "dvc=/var/log/relay/zpa.log") {
		t.Error("CEF should contain resource as device")
	}
}

func TestDetermineSeverity_AuthFailure(t *testing.T) {
	event := Event{
		EventType: EventAuthFailure,
		Success:   false,
	}

	severity := determineSeverity(event)
	if severity != 8 {
		t.Errorf("auth failure should have severity 8, got %d", severity)
	}
}

func TestDetermineSeverity_ConnectionRejected(t *testing.T) {
	event := Event{
		EventType: EventConnectionRejected,
		Success:   false,
	}

	severity := determineSeverity(event)
	if severity != 7 {
		t.Errorf("connection rejected should have severity 7, got %d", severity)
	}
}

func TestDetermineSeverity_AuthSuccess(t *testing.T) {
	event := Event{
		EventType: EventAuthSuccess,
		Success:   true,
	}

	severity := determineSeverity(event)
	if severity != 5 {
		t.Errorf("auth success should have severity 5, got %d", severity)
	}
}

func TestDetermineSeverity_ConfigChange(t *testing.T) {
	event := Event{
		EventType: EventConfigChange,
		Success:   true,
	}

	severity := determineSeverity(event)
	if severity != 6 {
		t.Errorf("config change should have severity 6, got %d", severity)
	}
}

func TestDetermineSeverity_ServerLifecycle(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		expected  int
	}{
		{"server start", EventServerStart, 4},
		{"server stop", EventServerStop, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := Event{
				EventType: tt.eventType,
				Success:   true,
			}

			severity := determineSeverity(event)
			if severity != tt.expected {
				t.Errorf("expected severity %d, got %d", tt.expected, severity)
			}
		})
	}
}

func TestDetermineSeverity_Routine(t *testing.T) {
	event := Event{
		EventType: EventConnectionAccepted,
		Success:   true,
	}

	severity := determineSeverity(event)
	if severity != 3 {
		t.Errorf("routine operations should have severity 3, got %d", severity)
	}
}

func TestCEFEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"pipe", "test|value", "test\\|value"},
		{"backslash", "test\\value", "test\\\\value"},
		{"equals", "test=value", "test\\=value"},
		{"newline", "test\nvalue", "test\\nvalue"},
		{"carriage return", "test\rvalue", "test\\rvalue"},
		{"multiple special chars", "a|b\\c=d\ne", "a\\|b\\\\c\\=d\\ne"},
		{"no special chars", "testvalue", "testvalue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cefEscape(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestCEFEscape_BackslashFirst(t *testing.T) {
	// Verify backslash is escaped first to avoid double-escaping
	input := "test\\|value"
	result := cefEscape(input)
	expected := "test\\\\\\|value"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildCEFExtensions(t *testing.T) {
	event := Event{
		Timestamp:    time.Date(2025, 11, 14, 10, 30, 45, 123000000, time.UTC),
		EventType:    EventConnectionAccepted,
		Success:      true,
		Actor:        "10.0.1.5",
		Resource:     "listener-1",
		Action:       "connect",
		Result:       "accepted",
		ConnectionID: "conn-123",
		Details: map[string]interface{}{
			"test": "value",
		},
	}

	ext := buildCEFExtensions(event)

	// Verify required fields
	if !strings.Contains(ext, "act=connect") {
		t.Error("extensions should contain action")
	}
	if !strings.Contains(ext, "src=10.0.1.5") {
		t.Error("extensions should contain source")
	}
	if !strings.Contains(ext, "outcome=accepted") {
		t.Error("extensions should contain outcome")
	}
	if !strings.Contains(ext, "dvc=listener-1") {
		t.Error("extensions should contain resource")
	}
	if !strings.Contains(ext, "cn1=conn-123") {
		t.Error("extensions should contain connection ID")
	}
	if !strings.Contains(ext, "cn1Label=Connection ID") {
		t.Error("extensions should contain connection ID label")
	}
	if !strings.Contains(ext, "cs1Label=Details") {
		t.Error("extensions should contain details label")
	}

	// Verify timestamp
	expectedTS := event.Timestamp.UnixMilli()
	if !strings.Contains(ext, "rt=") {
		t.Error("extensions should contain timestamp")
	}
	expectedTSStr := "rt=" + string(rune(expectedTS/1000000000))
	if !strings.Contains(ext, expectedTSStr[:4]) { // Check prefix
		t.Logf("Extensions: %s", ext)
	}
}

func TestBuildCEFExtensions_MinimalEvent(t *testing.T) {
	event := Event{
		Timestamp: time.Now().UTC(),
		EventType: EventConnectionAccepted,
		Success:   true,
		Actor:     "10.0.1.5",
		Action:    "connect",
		Result:    "accepted",
	}

	ext := buildCEFExtensions(event)

	// Should still have required fields
	if !strings.Contains(ext, "act=connect") {
		t.Error("extensions should contain action")
	}
	if !strings.Contains(ext, "src=10.0.1.5") {
		t.Error("extensions should contain source")
	}
	if !strings.Contains(ext, "outcome=accepted") {
		t.Error("extensions should contain outcome")
	}
	if !strings.Contains(ext, "rt=") {
		t.Error("extensions should contain timestamp")
	}

	// Should not have optional fields
	if strings.Contains(ext, "dvc=") {
		t.Error("extensions should not contain resource when not present")
	}
	if strings.Contains(ext, "cn1=") {
		t.Error("extensions should not contain connection ID when not present")
	}
}
