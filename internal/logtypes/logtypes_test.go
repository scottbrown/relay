package logtypes

import "testing"

func TestLogType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		logType  LogType
		expected bool
	}{
		{"user-activity is valid", UserActivity, true},
		{"user-status is valid", UserStatus, true},
		{"app-connector-status is valid", AppConnectorStatus, true},
		{"pse-status is valid", PSEStatus, true},
		{"browser-access is valid", BrowserAccess, true},
		{"audit is valid", Audit, true},
		{"app-connector-metrics is valid", AppConnectorMetrics, true},
		{"pse-metrics is valid", PSEMetrics, true},
		{"invalid log type", LogType("invalid"), false},
		{"empty log type", LogType(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.logType.IsValid(); got != tt.expected {
				t.Errorf("LogType.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogType_DefaultFilePrefix(t *testing.T) {
	tests := []struct {
		name     string
		logType  LogType
		expected string
	}{
		{"user-activity", UserActivity, "zpa-user-activity"},
		{"user-status", UserStatus, "zpa-user-status"},
		{"app-connector-status", AppConnectorStatus, "zpa-app-connector-status"},
		{"pse-status", PSEStatus, "zpa-pse-status"},
		{"browser-access", BrowserAccess, "zpa-browser-access"},
		{"audit", Audit, "zpa-audit"},
		{"app-connector-metrics", AppConnectorMetrics, "zpa-app-connector-metrics"},
		{"pse-metrics", PSEMetrics, "zpa-pse-metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.logType.DefaultFilePrefix(); got != tt.expected {
				t.Errorf("LogType.DefaultFilePrefix() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogType_DefaultSourceType(t *testing.T) {
	tests := []struct {
		name     string
		logType  LogType
		expected string
	}{
		{"user-activity", UserActivity, "zpa:user:activity"},
		{"user-status", UserStatus, "zpa:user:status"},
		{"app-connector-status", AppConnectorStatus, "zpa:app-connector:status"},
		{"pse-status", PSEStatus, "zpa:pse:status"},
		{"browser-access", BrowserAccess, "zpa:browser:access"},
		{"audit", Audit, "zpa:audit"},
		{"app-connector-metrics", AppConnectorMetrics, "zpa:app-connector:metrics"},
		{"pse-metrics", PSEMetrics, "zpa:pse:metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.logType.DefaultSourceType(); got != tt.expected {
				t.Errorf("LogType.DefaultSourceType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLogType_String(t *testing.T) {
	lt := UserActivity
	if got := lt.String(); got != "user-activity" {
		t.Errorf("LogType.String() = %v, want %v", got, "user-activity")
	}
}
