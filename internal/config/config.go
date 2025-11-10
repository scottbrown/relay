package config

import (
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/scottbrown/relay/internal/logtypes"
	"gopkg.in/yaml.v3"
)

const (
	DefaultMaxLineBytes       int    = 1 << 20 // 1 MiB
	DefaultHealthCheckAddr    string = ":9099"
	DefaultHealthCheckEnabled bool   = false
)

//go:embed config.template.yml
var configTemplate string

// SplunkConfig represents Splunk HEC configuration
type SplunkConfig struct {
	HECURL     string `yaml:"hec_url"`
	HECToken   string `yaml:"hec_token"`
	Gzip       *bool  `yaml:"gzip"`
	SourceType string `yaml:"source_type"`
}

// TLSConfig represents TLS configuration
type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

// ListenerConfig represents a single listener configuration
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

// Config represents the complete application configuration
type Config struct {
	Splunk             *SplunkConfig    `yaml:"splunk"`
	HealthCheckEnabled bool             `yaml:"health_check_enabled"`
	HealthCheckAddr    string           `yaml:"health_check_addr"`
	Listeners          []ListenerConfig `yaml:"listeners"`
}

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

	log.Printf("Loaded configuration from: %s", configFile)
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

		// Validate TLS configuration (both cert and key must be present or both absent)
		if listener.TLS != nil {
			if (listener.TLS.CertFile == "") != (listener.TLS.KeyFile == "") {
				return fmt.Errorf("listener %s: both tls.cert_file and tls.key_file must be specified or both omitted", listener.Name)
			}
			if listener.TLS.CertFile != "" {
				// Check if cert file exists
				if _, err := os.Stat(listener.TLS.CertFile); os.IsNotExist(err) {
					return fmt.Errorf("listener %s: tls.cert_file not found: %s", listener.Name, listener.TLS.CertFile)
				}
				// Check if key file exists
				if _, err := os.Stat(listener.TLS.KeyFile); os.IsNotExist(err) {
					return fmt.Errorf("listener %s: tls.key_file not found: %s", listener.Name, listener.TLS.KeyFile)
				}
			}
		}

		// Validate Splunk configuration (requires sourcetype if HEC is configured)
		if listener.Splunk != nil {
			if listener.Splunk.SourceType == "" && (listener.Splunk.HECURL != "" || listener.Splunk.HECToken != "") {
				return fmt.Errorf("listener %s: splunk.source_type is required when HEC is configured", listener.Name)
			}
		} else if cfg.Splunk != nil {
			// If using global Splunk config, listener must have a sourcetype
			if cfg.Splunk.HECURL != "" || cfg.Splunk.HECToken != "" {
				if listener.Splunk == nil || listener.Splunk.SourceType == "" {
					return fmt.Errorf("listener %s: splunk.source_type is required when global HEC is configured", listener.Name)
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

func GetTemplate() string {
	return configTemplate
}
