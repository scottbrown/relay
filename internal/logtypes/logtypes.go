// Package logtypes defines Zscaler ZPA log types and provides helper functions
// for file naming and Splunk sourcetype mapping.
package logtypes

import "strings"

// LogType represents a Zscaler ZPA log type.
// Each log type corresponds to a specific category of logs from ZPA LSS.
type LogType string

const (
	// UserActivity logs contain user session and access activity.
	UserActivity LogType = "user-activity"
	// UserStatus logs contain user connection status information.
	UserStatus LogType = "user-status"
	// AppConnectorStatus logs contain App Connector health and status information.
	AppConnectorStatus LogType = "app-connector-status"
	// PSEStatus logs contain Private Service Edge status information.
	PSEStatus LogType = "pse-status"
	// BrowserAccess logs contain browser-based access activity.
	BrowserAccess LogType = "browser-access"
	// Audit logs contain administrative and configuration audit events.
	Audit LogType = "audit"
	// AppConnectorMetrics logs contain App Connector performance metrics.
	AppConnectorMetrics LogType = "app-connector-metrics"
	// PSEMetrics logs contain Private Service Edge performance metrics.
	PSEMetrics LogType = "pse-metrics"
)

// IsValid returns true if the log type matches one of the defined ZPA log types.
func (lt LogType) IsValid() bool {
	switch lt {
	case UserActivity, UserStatus, AppConnectorStatus, PSEStatus,
		BrowserAccess, Audit, AppConnectorMetrics, PSEMetrics:
		return true
	}
	return false
}

// DefaultFilePrefix returns the default file prefix for this log type.
// The prefix follows the pattern "zpa-{log-type}" for use in storage file naming.
func (lt LogType) DefaultFilePrefix() string {
	return "zpa-" + string(lt)
}

// DefaultSourceType returns the default Splunk sourcetype for this log type.
// Sourcetypes follow the pattern "zpa:category:subcategory" using colons as separators.
func (lt LogType) DefaultSourceType() string {
	s := string(lt)
	// Special handling for app-connector types
	if strings.HasPrefix(s, "app-connector-") {
		// Keep "app-connector" intact, only replace the last hyphen
		parts := strings.Split(s, "-")
		if len(parts) == 3 {
			return "zpa:app-connector:" + parts[2]
		}
	}
	// For all other types, replace hyphens with colons
	return "zpa:" + strings.ReplaceAll(s, "-", ":")
}

// String returns the string representation of the log type.
func (lt LogType) String() string {
	return string(lt)
}
