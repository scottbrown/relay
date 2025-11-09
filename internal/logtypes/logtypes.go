package logtypes

import "strings"

// LogType represents a ZPA log type
type LogType string

const (
	UserActivity        LogType = "user-activity"
	UserStatus          LogType = "user-status"
	AppConnectorStatus  LogType = "app-connector-status"
	PSEStatus           LogType = "pse-status"
	BrowserAccess       LogType = "browser-access"
	Audit               LogType = "audit"
	AppConnectorMetrics LogType = "app-connector-metrics"
	PSEMetrics          LogType = "pse-metrics"
)

// IsValid returns true if the log type is valid
func (lt LogType) IsValid() bool {
	switch lt {
	case UserActivity, UserStatus, AppConnectorStatus, PSEStatus,
		BrowserAccess, Audit, AppConnectorMetrics, PSEMetrics:
		return true
	}
	return false
}

// DefaultFilePrefix returns the default file prefix for this log type
func (lt LogType) DefaultFilePrefix() string {
	return "zpa-" + string(lt)
}

// DefaultSourceType returns the default Splunk sourcetype for this log type
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

// String returns the string representation of the log type
func (lt LogType) String() string {
	return string(lt)
}
