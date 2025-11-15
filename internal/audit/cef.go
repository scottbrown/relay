package audit

import (
	"fmt"
	"strings"

	"github.com/scottbrown/relay"
)

// formatCEF formats an audit event in Common Event Format (CEF) for SIEM integration.
// CEF Format: CEF:Version|Device Vendor|Device Product|Device Version|Signature ID|Name|Severity|Extension
// Reference: https://www.microfocus.com/documentation/arcsight/arcsight-smartconnectors-8.3/cef-implementation-standard/
func formatCEF(event Event) []byte {
	// Determine severity based on event outcome
	severity := determineSeverity(event)

	// Build CEF header
	header := fmt.Sprintf("CEF:0|Relay|ZPA Log Relay|%s|%s|%s|%d",
		relay.Version(),
		cefEscape(string(event.EventType)),
		cefEscape(event.Action),
		severity,
	)

	// Build extension fields
	extensions := buildCEFExtensions(event)

	return []byte(header + "|" + extensions)
}

// determineSeverity maps event outcomes to CEF severity levels (0-10).
func determineSeverity(event Event) int {
	// Failed security events are high severity
	if !event.Success {
		switch event.EventType {
		case EventAuthFailure:
			return 8 // High severity for auth failures
		case EventConnectionRejected:
			return 7 // Medium-high for connection rejections
		default:
			return 6 // Medium for other failures
		}
	}

	// Successful events have lower severity
	switch event.EventType {
	case EventAuthSuccess:
		return 5 // Medium for successful auth
	case EventConfigChange:
		return 6 // Medium for config changes
	case EventServerStart, EventServerStop:
		return 4 // Low-medium for server lifecycle
	default:
		return 3 // Low for routine operations
	}
}

// buildCEFExtensions creates the extension field string for CEF format.
// Uses CEF standard field names where possible.
func buildCEFExtensions(event Event) string {
	var parts []string

	// Standard CEF fields
	parts = append(parts, fmt.Sprintf("act=%s", cefEscape(event.Action)))
	parts = append(parts, fmt.Sprintf("src=%s", cefEscape(event.Actor)))
	parts = append(parts, fmt.Sprintf("outcome=%s", cefEscape(event.Result)))

	// Add resource if present
	if event.Resource != "" {
		parts = append(parts, fmt.Sprintf("dvc=%s", cefEscape(event.Resource)))
	}

	// Add connection ID if present
	if event.ConnectionID != "" {
		parts = append(parts, fmt.Sprintf("cn1=%s", cefEscape(event.ConnectionID)))
		parts = append(parts, "cn1Label=Connection ID")
	}

	// Add details as custom fields
	if len(event.Details) > 0 {
		for key, value := range event.Details {
			parts = append(parts, fmt.Sprintf("cs1=%s", cefEscape(fmt.Sprintf("%s=%v", key, value))))
		}
		parts = append(parts, "cs1Label=Details")
	}

	// Add timestamp in CEF format (milliseconds since epoch)
	parts = append(parts, fmt.Sprintf("rt=%d", event.Timestamp.UnixMilli()))

	return strings.Join(parts, " ")
}

// cefEscape escapes special characters in CEF field values.
// CEF requires escaping of pipe (|), backslash (\), equals (=), newline, and carriage return.
func cefEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\") // Backslash must be escaped first
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "=", "\\=")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}
