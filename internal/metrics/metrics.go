package metrics

import (
	"expvar"
	"time"
)

var (
	// Connection metrics
	ConnectionsAccepted = expvar.NewInt("connections_accepted")
	ConnectionsRejected = expvar.NewInt("connections_rejected")
	ConnectionsActive   = expvar.NewInt("connections_active")
	BytesReceived       = expvar.NewInt("bytes_received_total")

	// Storage metrics
	StorageWrites        = expvar.NewMap("storage_writes")
	StorageBytesWritten  = expvar.NewInt("storage_bytes_written")
	StorageFileRotations = expvar.NewInt("storage_file_rotations")

	// HEC forwarder metrics
	HecForwards       = expvar.NewMap("hec_forwards")
	HecBytesForwarded = expvar.NewInt("hec_bytes_forwarded")
	HecRetries        = expvar.NewInt("hec_retries_total")

	// Processing metrics
	LinesProcessed = expvar.NewMap("lines_processed")

	// System metrics
	StartTime = expvar.NewInt("start_time_seconds")
	Version   = expvar.NewString("version_info")
)

// Init initialises system metrics that should be set once at startup.
func Init(versionString string) {
	StartTime.Set(time.Now().Unix())
	Version.Set(versionString)
}
