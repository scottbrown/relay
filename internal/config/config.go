// Package config handles loading and validation of application configuration.
// It supports YAML-based configuration files and provides sensible defaults.
package config

import (
	"crypto/tls"
	_ "embed"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/scottbrown/relay/internal/acl"
	"github.com/scottbrown/relay/internal/logtypes"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultMaxLineBytes is the default maximum size for a single log line (1 MiB).
	DefaultMaxLineBytes int = 1 << 20
	// DefaultHealthCheckAddr is the default address for the health check server.
	DefaultHealthCheckAddr string = ":9099"
	// DefaultHealthCheckEnabled indicates if health checks are enabled by default.
	DefaultHealthCheckEnabled bool = false
)

//go:embed config.template.yml
var configTemplate string

// BatchConfig holds configuration for batching multiple log lines before forwarding.
// When enabled, logs are accumulated and sent together to reduce network overhead.
type BatchConfig struct {
	Enabled       *bool `yaml:"enabled"`
	MaxSize       int   `yaml:"max_size"`
	MaxBytes      int   `yaml:"max_bytes"`
	FlushInterval int   `yaml:"flush_interval_seconds"`
}

// CircuitBreakerConfig holds configuration for the circuit breaker pattern.
// The circuit breaker prevents cascading failures by temporarily stopping
// requests to a failing service.
type CircuitBreakerConfig struct {
	Enabled          *bool `yaml:"enabled"`
	FailureThreshold int   `yaml:"failure_threshold"`
	SuccessThreshold int   `yaml:"success_threshold"`
	Timeout          int   `yaml:"timeout_seconds"`
	HalfOpenMaxCalls int   `yaml:"half_open_max_calls"`
}

// RetryConfig holds configuration for retry behaviour with exponential backoff.
// These parameters control how many times the forwarder will retry failed HEC requests
// and how long it will wait between attempts.
type RetryConfig struct {
	MaxAttempts       int     `yaml:"max_attempts"`
	InitialBackoffMS  int     `yaml:"initial_backoff_ms"`
	BackoffMultiplier float64 `yaml:"backoff_multiplier"`
	MaxBackoffSeconds int     `yaml:"max_backoff_seconds"`
}

// HECTarget represents a single Splunk HEC endpoint target.
// Multiple targets can be configured for high availability or multi-tenancy.
type HECTarget struct {
	Name           string                `yaml:"name"`
	HECURL         string                `yaml:"hec_url"`
	HECToken       string                `yaml:"hec_token"`
	Gzip           *bool                 `yaml:"gzip"`
	SourceType     string                `yaml:"source_type"`
	Batch          *BatchConfig          `yaml:"batch"`
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker"`
	Retry          *RetryConfig          `yaml:"retry"`
}

// RoutingMode defines how logs are distributed across multiple HEC targets.
type RoutingMode string

const (
	// RoutingModeAll sends logs to all configured targets (broadcast).
	RoutingModeAll RoutingMode = "all"
	// RoutingModePrimaryFailover tries primary target first, fails over to secondary on error.
	RoutingModePrimaryFailover RoutingMode = "primary-failover"
	// RoutingModeRoundRobin distributes logs across targets in round-robin fashion.
	RoutingModeRoundRobin RoutingMode = "round-robin"
)

// RoutingConfig holds routing configuration for multiple HEC targets.
type RoutingConfig struct {
	Mode RoutingMode `yaml:"mode"`
}

// SplunkConfig holds Splunk HEC (HTTP Event Collector) configuration.
// It includes connection details, batching, and circuit breaker settings.
// Supports both legacy single target configuration and new multi-target configuration.
type SplunkConfig struct {
	// Legacy single target configuration
	HECURL         string                `yaml:"hec_url"`
	HECToken       string                `yaml:"hec_token"`
	Gzip           *bool                 `yaml:"gzip"`
	SourceType     string                `yaml:"source_type"`
	Batch          *BatchConfig          `yaml:"batch"`
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker"`
	Retry          *RetryConfig          `yaml:"retry"`

	// Multi-target configuration
	HECTargets []HECTarget    `yaml:"hec_targets"`
	Routing    *RoutingConfig `yaml:"routing"`
}

// TLSConfig holds TLS certificate configuration for encrypted connections.
// Both CertFile and KeyFile must be specified together.
type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// ListenerConfig holds configuration for a single TCP listener.
// Each listener can accept ZPA logs on a specific port and handle a specific log type.
type ListenerConfig struct {
	Name         string        `yaml:"name"`
	ListenAddr   string        `yaml:"listen_addr"`
	LogType      string        `yaml:"log_type"`
	OutputDir    string        `yaml:"output_dir"`
	FilePrefix   string        `yaml:"file_prefix"`
	TLS          *TLSConfig    `yaml:"tls"`
	AllowedCIDRs string        `yaml:"allowed_cidrs"`
	MaxLineBytes int           `yaml:"max_line_bytes"`
	Splunk       *SplunkConfig `yaml:"splunk"`
}

// Config represents the complete application configuration.
// It supports multiple listeners, each with independent settings for storage and forwarding.
type Config struct {
	Splunk             *SplunkConfig    `yaml:"splunk"`
	HealthCheckEnabled bool             `yaml:"health_check_enabled"`
	HealthCheckAddr    string           `yaml:"health_check_addr"`
	Listeners          []ListenerConfig `yaml:"listeners"`
}

// LoadConfig reads and validates configuration from the specified YAML file.
// It returns an error if the file cannot be read, parsed, or contains invalid settings.
// All listener addresses, TLS certificates, and storage directories are validated during load.
func LoadConfig(configFile string) (*Config, error) {
	// Config file is now required
	if configFile == "" {
		return nil, fmt.Errorf("configuration file is required")
	}

	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configFile)
	}

	// Read config file
	// #nosec G304 -- configFile is provided by the user via the --config flag, which is the
	// expected and documented way to specify the configuration file path.
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse YAML
	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %v", err)
	}

	// Apply defaults
	if config.HealthCheckAddr == "" {
		config.HealthCheckAddr = DefaultHealthCheckAddr
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	slog.Info("loaded configuration", "file", configFile)
	return config, nil
}

func validateConfig(cfg *Config) error {
	// Require at least one listener
	if len(cfg.Listeners) == 0 {
		return fmt.Errorf("at least one listener is required")
	}

	// Track unique listen addresses
	listenAddrs := make(map[string]bool)

	for i, listener := range cfg.Listeners {
		// Validate required fields
		if listener.Name == "" {
			return fmt.Errorf("listener %d: name is required", i)
		}
		if listener.ListenAddr == "" {
			return fmt.Errorf("listener %s: listen_addr is required", listener.Name)
		}
		if listener.LogType == "" {
			return fmt.Errorf("listener %s: log_type is required", listener.Name)
		}
		if listener.OutputDir == "" {
			return fmt.Errorf("listener %s: output_dir is required", listener.Name)
		}
		if listener.FilePrefix == "" {
			return fmt.Errorf("listener %s: file_prefix is required", listener.Name)
		}

		// Validate log type
		lt := logtypes.LogType(listener.LogType)
		if !lt.IsValid() {
			return fmt.Errorf("listener %s: invalid log_type '%s'", listener.Name, listener.LogType)
		}

		// Check for duplicate listen addresses
		if listenAddrs[listener.ListenAddr] {
			return fmt.Errorf("listener %s: duplicate listen_addr '%s'", listener.Name, listener.ListenAddr)
		}
		listenAddrs[listener.ListenAddr] = true

		// Validate listen address availability
		if err := validateListenAddr(listener.ListenAddr); err != nil {
			return fmt.Errorf("listener %s: cannot bind to listen address: %w", listener.Name, err)
		}

		// Validate TLS configuration
		if listener.TLS != nil {
			if (listener.TLS.CertFile == "") != (listener.TLS.KeyFile == "") {
				return fmt.Errorf("listener %s: both tls.cert_file and tls.key_file must be specified or both omitted", listener.Name)
			}
			if listener.TLS.CertFile != "" {
				// Check if cert file exists and is readable
				if _, err := os.Stat(listener.TLS.CertFile); err != nil {
					return fmt.Errorf("listener %s: TLS cert file not accessible: %w", listener.Name, err)
				}
				// Check if key file exists and is readable
				if _, err := os.Stat(listener.TLS.KeyFile); err != nil {
					return fmt.Errorf("listener %s: TLS key file not accessible: %w", listener.Name, err)
				}
				// Validate certificate by loading it
				if _, err := tls.LoadX509KeyPair(listener.TLS.CertFile, listener.TLS.KeyFile); err != nil {
					return fmt.Errorf("listener %s: failed to load TLS certificate: %w", listener.Name, err)
				}
			}
		}

		// Validate storage directory (create if needed and test writability)
		if err := validateStorageDir(listener.OutputDir); err != nil {
			return fmt.Errorf("listener %s: %w", listener.Name, err)
		}

		// Validate CIDR list
		if listener.AllowedCIDRs != "" {
			if _, err := acl.New(listener.AllowedCIDRs); err != nil {
				return fmt.Errorf("listener %s: invalid CIDR list: %w", listener.Name, err)
			}
		}

		// Merge and validate Splunk configuration
		hecURL := ""
		hecToken := ""
		sourceType := ""
		hasMultiTarget := false

		// Start with global config
		if cfg.Splunk != nil {
			hecURL = cfg.Splunk.HECURL
			hecToken = cfg.Splunk.HECToken
			if len(cfg.Splunk.HECTargets) > 0 {
				hasMultiTarget = true
			}
		}

		// Override with per-listener config
		if listener.Splunk != nil {
			if listener.Splunk.HECURL != "" {
				hecURL = listener.Splunk.HECURL
			}
			if listener.Splunk.HECToken != "" {
				hecToken = listener.Splunk.HECToken
			}
			if listener.Splunk.SourceType != "" {
				sourceType = listener.Splunk.SourceType
			}
			if len(listener.Splunk.HECTargets) > 0 {
				hasMultiTarget = true
			}
		}

		// Validate single vs multi-target configuration
		if hasMultiTarget {
			// Validate multi-target configuration
			if err := validateMultiTargetConfig(cfg.Splunk, listener.Splunk, listener.Name); err != nil {
				return err
			}
		} else {
			// Validate legacy single HEC configuration
			if hecURL != "" || hecToken != "" {
				// Both URL and token must be specified
				if hecURL == "" {
					return fmt.Errorf("listener %s: HEC URL required when HEC token is specified", listener.Name)
				}
				if hecToken == "" {
					return fmt.Errorf("listener %s: HEC token required when HEC URL is specified", listener.Name)
				}
				if sourceType == "" {
					return fmt.Errorf("listener %s: splunk.source_type is required when HEC is configured", listener.Name)
				}

				// Validate HEC URL format
				if err := validateHECURL(hecURL); err != nil {
					return fmt.Errorf("listener %s: invalid HEC URL: %w", listener.Name, err)
				}
			}
		}

		// Apply default max line bytes if not specified
		if listener.MaxLineBytes == 0 {
			cfg.Listeners[i].MaxLineBytes = DefaultMaxLineBytes
		}
	}

	return nil
}

// validateListenAddr verifies that the listen address is valid and available
func validateListenAddr(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	_ = ln.Close() // Ignore error - best effort cleanup
	return nil
}

// validateStorageDir ensures the storage directory exists and is writable
func validateStorageDir(dir string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	// Verify directory is writable by creating a temp file
	testFile := filepath.Join(dir, ".writetest")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return fmt.Errorf("output directory not writable: %w", err)
	}
	_ = os.Remove(testFile) // Ignore error - best effort cleanup

	return nil
}

// validateHECURL validates the HEC URL format
func validateHECURL(hecURL string) error {
	u, err := url.Parse(hecURL)
	if err != nil {
		return err
	}

	// Must be HTTP/HTTPS
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("HEC URL must use http or https scheme")
	}

	// Host must be specified
	if u.Host == "" {
		return fmt.Errorf("HEC URL must include host")
	}

	return nil
}

// GetTemplate returns the embedded YAML configuration template.
// This template can be used to generate a sample configuration file.
func GetTemplate() string {
	return configTemplate
}

// validateMultiTargetConfig validates multi-target HEC configuration
func validateMultiTargetConfig(global, perListener *SplunkConfig, listenerName string) error {
	// Collect targets from both global and per-listener config
	var targets []HECTarget

	if global != nil && len(global.HECTargets) > 0 {
		// Cannot mix single and multi-target config at global level
		if global.HECURL != "" || global.HECToken != "" {
			return fmt.Errorf("listener %s: cannot specify both legacy HEC config (hec_url/hec_token) and hec_targets", listenerName)
		}
		targets = append(targets, global.HECTargets...)
	}

	if perListener != nil && len(perListener.HECTargets) > 0 {
		// Cannot mix single and multi-target config at listener level
		if perListener.HECURL != "" || perListener.HECToken != "" {
			return fmt.Errorf("listener %s: cannot specify both legacy HEC config (hec_url/hec_token) and hec_targets", listenerName)
		}
		// Per-listener targets override global targets
		targets = perListener.HECTargets
	}

	// Require at least one target
	if len(targets) == 0 {
		return fmt.Errorf("listener %s: at least one HEC target required when using multi-target configuration", listenerName)
	}

	// Track unique target names
	targetNames := make(map[string]bool)

	// Validate each target
	for i, target := range targets {
		if target.Name == "" {
			return fmt.Errorf("listener %s: target %d: name is required", listenerName, i)
		}

		// Check for duplicate names
		if targetNames[target.Name] {
			return fmt.Errorf("listener %s: duplicate target name '%s'", listenerName, target.Name)
		}
		targetNames[target.Name] = true

		if target.HECURL == "" {
			return fmt.Errorf("listener %s: target '%s': hec_url is required", listenerName, target.Name)
		}
		if target.HECToken == "" {
			return fmt.Errorf("listener %s: target '%s': hec_token is required", listenerName, target.Name)
		}
		if target.SourceType == "" {
			return fmt.Errorf("listener %s: target '%s': source_type is required", listenerName, target.Name)
		}

		// Validate HEC URL format
		if err := validateHECURL(target.HECURL); err != nil {
			return fmt.Errorf("listener %s: target '%s': invalid HEC URL: %w", listenerName, target.Name, err)
		}
	}

	// Validate routing configuration
	var routingMode RoutingMode
	if perListener != nil && perListener.Routing != nil {
		routingMode = perListener.Routing.Mode
	} else if global != nil && global.Routing != nil {
		routingMode = global.Routing.Mode
	} else {
		// Default routing mode
		routingMode = RoutingModeAll
	}

	// Validate routing mode
	if !isValidRoutingMode(routingMode) {
		return fmt.Errorf("listener %s: invalid routing mode '%s' (must be one of: all, primary-failover, round-robin)", listenerName, routingMode)
	}

	return nil
}

// isValidRoutingMode checks if the routing mode is valid
func isValidRoutingMode(mode RoutingMode) bool {
	switch mode {
	case RoutingModeAll, RoutingModePrimaryFailover, RoutingModeRoundRobin:
		return true
	default:
		return false
	}
}
