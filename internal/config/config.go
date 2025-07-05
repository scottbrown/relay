package config

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	DefaultListenPort   string        = "9514"
	DefaultSourceType   string        = "zscaler:zpa:lss"
	DefaultIndex        string        = "main"
	DefaultBatchSize    int           = 100
	DefaultBatchTimeout time.Duration = 5 * time.Second
)

//go:embed config.template.yml
var configTemplate string

type Config struct {
	ListenPort   string        `yaml:"listen_port"`
	SplunkHECURL string        `yaml:"splunk_hec_url"`
	SplunkToken  string        `yaml:"splunk_token"`
	SourceType   string        `yaml:"source_type"`
	Index        string        `yaml:"index"`
	BatchSize    int           `yaml:"batch_size"`
	BatchTimeout time.Duration `yaml:"batch_timeout"`
}

func LoadConfig() (*Config, error) {
	var configFile string
	flag.StringVar(&configFile, "f", "/etc/relay/config.yml", "Path to configuration file")
	flag.Parse()

	// Set default values
	config := &Config{
		ListenPort:   DefaultListenPort,
		SourceType:   DefaultSourceType,
		Index:        DefaultIndex,
		BatchSize:    DefaultBatchSize,
		BatchTimeout: DefaultBatchTimeout,
	}

	// Check if config file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s", configFile)
	}

	// Read config file
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse YAML
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %v", err)
	}

	// Validate required fields
	if config.SplunkHECURL == "" {
		return nil, fmt.Errorf("splunk_hec_url is required in config file")
	}
	if config.SplunkToken == "" {
		return nil, fmt.Errorf("splunk_token is required in config file")
	}

	log.Printf("Loaded configuration from: %s", configFile)
	return config, nil
}

func GetTemplate() string {
	return configTemplate
}
