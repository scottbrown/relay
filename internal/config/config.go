package config

import (
	"crypto/tls"
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"

	"github.com/scottbrown/relay/internal/acl"
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

		// Start with global config
		if cfg.Splunk != nil {
			hecURL = cfg.Splunk.HECURL
			hecToken = cfg.Splunk.HECToken
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
		}

		// Validate HEC configuration
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
	ln.Close()
	return nil
}

// validateStorageDir ensures the storage directory exists and is writable
func validateStorageDir(dir string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}

	// Verify directory is writable by creating a temp file
	testFile := filepath.Join(dir, ".writetest")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("output directory not writable: %w", err)
	}
	os.Remove(testFile)

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

func GetTemplate() string {
	return configTemplate
}
