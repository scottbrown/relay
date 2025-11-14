package config

import (
	_ "embed"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	DefaultListenAddr         string = ":9015"
	DefaultOutputDir          string = "./zpa-logs"
	DefaultSourceType         string = "zpa:lss"
	DefaultMaxLineBytes       int    = 1 << 20 // 1 MiB
	DefaultHealthCheckAddr    string = ":9016"
	DefaultHealthCheckEnabled bool   = false
)

//go:embed config.template.yml
var configTemplate string

type Config struct {
	ListenAddr         string `yaml:"listen_addr"`
	TLSCertFile        string `yaml:"tls_cert_file"`
	TLSKeyFile         string `yaml:"tls_key_file"`
	OutputDir          string `yaml:"output_dir"`
	SplunkHECURL       string `yaml:"splunk_hec_url"`
	SplunkToken        string `yaml:"splunk_token"`
	SourceType         string `yaml:"source_type"`
	AllowedCIDRs       string `yaml:"allowed_cidrs"`
	GzipHEC            bool   `yaml:"gzip_hec"`
	MaxLineBytes       int    `yaml:"max_line_bytes"`
	HealthCheckEnabled bool   `yaml:"health_check_enabled"`
	HealthCheckAddr    string `yaml:"health_check_addr"`
}

func LoadConfig(configFile string) (*Config, error) {
	// Set default values
	config := &Config{
		ListenAddr:         DefaultListenAddr,
		OutputDir:          DefaultOutputDir,
		SourceType:         DefaultSourceType,
		GzipHEC:            true,
		MaxLineBytes:       DefaultMaxLineBytes,
		HealthCheckEnabled: DefaultHealthCheckEnabled,
		HealthCheckAddr:    DefaultHealthCheckAddr,
	}

	// If no config file specified, return defaults
	if configFile == "" {
		return config, nil
	}

	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configFile)
	}

	// Read config file
	// #nosec G304 - Config file path is provided by user via CLI flag, this is expected behaviour
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %v", err)
	}

	log.Printf("Loaded configuration from: %s", configFile)
	return config, nil
}

func GetTemplate() string {
	return configTemplate
}
