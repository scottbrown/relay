// Package relay implements a TCP relay service for Zscaler ZPA LSS logs.
// It receives log data over TCP/TLS, persists it locally, and forwards to Splunk HEC.
package relay

import (
	"fmt"
)

var (
	version string
	build   string
)

// Version returns the application version and build information.
// The version and build values are injected at compile time via ldflags.
func Version() string {
	return fmt.Sprintf("%s (%s)", version, build)
}
