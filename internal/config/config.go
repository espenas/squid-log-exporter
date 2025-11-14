package config

import (
	"fmt"
	"os"
	"regexp"
	"sort"

	"gopkg.in/yaml.v3"
)

// Config represents the exporter configuration
type Config struct {
	Global           GlobalConfig      `yaml:"global"`
	LogFormat        LogFormatConfig   `yaml:"log_format"`
	MonitoredDomains []MonitoredDomain `yaml:"monitored_domains"`
	DomainPatterns   []DomainPattern   `yaml:"domain_patterns"`
}

// GlobalConfig contains global settings
type GlobalConfig struct {
	TrackAllDomains bool `yaml:"track_all_domains"`
	MaxDomains      int  `yaml:"max_domains"`
}

// LogFormatConfig defines the log format
type LogFormatConfig struct {
	Type            string         `yaml:"type"`
	Fields          map[string]int `yaml:"fields,omitempty"`
	TimestampFormat string         `yaml:"timestamp_format,omitempty"`
	DurationUnit    string         `yaml:"duration_unit,omitempty"`
}

// MonitoredDomain represents a domain with extended monitoring
type MonitoredDomain struct {
	Host   string            `yaml:"host"`
	Port   string            `yaml:"port"`
	Labels map[string]string `yaml:"labels"`
}

// DomainPattern represents a pattern-based domain configuration
type DomainPattern struct {
	Pattern string            `yaml:"pattern"`
	Labels  map[string]string `yaml:"labels"`
	regex   *regexp.Regexp
}

// Predefined log format presets
var logFormatPresets = map[string]LogFormatConfig{
	"squid_native": {
		Type: "squid_native",
		Fields: map[string]int{
			"timestamp":    0,
			"duration":     1,
			"client_ip":    2,
			"result_code":  3,
			"bytes":        4,
			"method":       5,
			"url":          6,
			"rfc931":       7,
			"hierarchy":    8,
			"content_type": 9,
		},
		TimestampFormat: "unix",
		DurationUnit:    "ms",
	},
	"squid_combined": {
		Type: "squid_combined",
		Fields: map[string]int{
			"timestamp":    0,
			"duration":     1,
			"client_ip":    2,
			"result_code":  3,
			"bytes":        4,
			"method":       5,
			"url":          6,
			"rfc931":       7,
			"hierarchy":    8,
			"content_type": 9,
			"referer":      10,
			"user_agent":   11,
		},
		TimestampFormat: "unix",
		DurationUnit:    "ms",
	},
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Global.MaxDomains == 0 {
		config.Global.MaxDomains = 10000
	}

	// Apply log format preset or defaults
	if err := config.applyLogFormatDefaults(); err != nil {
		return nil, err
	}

	// Compile regex patterns
	for i := range config.DomainPatterns {
		pattern := config.DomainPatterns[i].Pattern
		regexPattern := "^" + regexp.QuoteMeta(pattern) + "$"
		regexPattern = regexp.MustCompile(`\\\*`).ReplaceAllString(regexPattern, ".*")

		regex, err := regexp.Compile(regexPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %s: %w", pattern, err)
		}
		config.DomainPatterns[i].regex = regex
	}

	return &config, nil
}

// applyLogFormatDefaults applies preset or default log format
func (c *Config) applyLogFormatDefaults() error {
	if c.LogFormat.Type == "" {
		c.LogFormat = logFormatPresets["squid_native"]
		return nil
	}

	if preset, exists := logFormatPresets[c.LogFormat.Type]; exists {
		if len(c.LogFormat.Fields) == 0 {
			c.LogFormat.Fields = preset.Fields
		}
		if c.LogFormat.TimestampFormat == "" {
			c.LogFormat.TimestampFormat = preset.TimestampFormat
		}
		if c.LogFormat.DurationUnit == "" {
			c.LogFormat.DurationUnit = preset.DurationUnit
		}
		return nil
	}

	if c.LogFormat.Type == "custom" {
		if len(c.LogFormat.Fields) == 0 {
			return fmt.Errorf("custom log format requires 'fields' to be defined")
		}

		if c.LogFormat.TimestampFormat == "" {
			c.LogFormat.TimestampFormat = "2006-01-02T15:04:05.000"
		}
		if c.LogFormat.DurationUnit == "" {
			c.LogFormat.DurationUnit = "ms"
		}

		requiredFields := []string{"timestamp", "duration", "result_code", "bytes", "method", "url"}
		for _, field := range requiredFields {
			if _, ok := c.LogFormat.Fields[field]; !ok {
				return fmt.Errorf("custom log format missing required field: %s", field)
			}
		}

		return nil
	}

	return fmt.Errorf("unknown log format type: %s (valid: squid_native, squid_combined, custom)", c.LogFormat.Type)
}

// GetField safely gets a field value from parsed line
func (c *LogFormatConfig) GetField(fields []string, fieldName string) string {
	if idx, ok := c.Fields[fieldName]; ok && idx < len(fields) {
		return fields[idx]
	}
	return ""
}

// IsMonitored checks if a domain should have extended monitoring
func (c *Config) IsMonitored(host, port string) (*MonitoredDomain, bool) {
	for _, domain := range c.MonitoredDomains {
		if domain.Host == host && (domain.Port == "" || domain.Port == port) {
			return &domain, true
		}
	}

	for _, pattern := range c.DomainPatterns {
		if pattern.regex.MatchString(host) {
			return &MonitoredDomain{
				Host:   host,
				Port:   port,
				Labels: pattern.Labels,
			}, true
		}
	}

	return nil, false
}

// GetCustomLabelKeys returns all unique custom label keys from config
func (c *Config) GetCustomLabelKeys() []string {
	keySet := make(map[string]bool)

	for _, domain := range c.MonitoredDomains {
		for key := range domain.Labels {
			keySet[key] = true
		}
	}

	for _, pattern := range c.DomainPatterns {
		for key := range pattern.Labels {
			keySet[key] = true
		}
	}

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}
