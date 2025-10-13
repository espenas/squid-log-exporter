/*
Copyright (C) 2024 Espen Stefansen <espenas+github@gmail.com>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "os"
    "time"

    "gopkg.in/yaml.v2"
)

// loadConfig loads configuration from file and command line flags
func loadConfig() (*Config, error) {
    // Define default configuration
    config := &Config{
        AccessLogPath:        "/var/log/squid/access.log",
        PositionFilePath:     "/var/lib/squid_exporter/position.txt",
        OutputPath:           "/var/lib/squid_exporter/metrics.txt",
        BufferSize:           65536,
        LogErrors:            true,
        RetryAttempts:        3,
        RetryDelay:           "1s",
        LogFilePath:          "/var/lib/squid_exporter/squid_metrics.log",
        KnownCodesFilePath:   "/var/lib/squid_exporter/known_http_codes.txt",
        KnownStatusFilePath:  "/var/lib/squid_exporter/known_status.txt",
        MonitoredDomainsPath: "",
    }

    // Parse command line flags
    flags := parseFlags()

    // If version flag is set, print version and exit
    if flags.Version {
        fmt.Println("Squid Log Exporter v1.1.0")
        os.Exit(0)
    }

    // If config file is specified, load it
    if flags.ConfigFile != "" {
        if err := loadConfigFile(flags.ConfigFile, config); err != nil {
            return nil, fmt.Errorf("error loading config file: %v", err)
        }
    }

    // Override with command line flags if they're set
    if flags.AccessLogPath != "" {
        config.AccessLogPath = flags.AccessLogPath
    }
    if flags.PositionFilePath != "" {
        config.PositionFilePath = flags.PositionFilePath
    }
    if flags.OutputPath != "" {
        config.OutputPath = flags.OutputPath
    }
    if flags.BufferSize != 0 {
        config.BufferSize = flags.BufferSize
    }
    if flags.LogErrors != nil {
        config.LogErrors = *flags.LogErrors
    }
    if flags.RetryAttempts != 0 {
        config.RetryAttempts = flags.RetryAttempts
    }
    if flags.RetryDelay != "" {
        config.RetryDelay = flags.RetryDelay
    }
    if flags.DomainsConfig != "" {
        config.MonitoredDomainsPath = flags.DomainsConfig
    }

    return config, nil
}

// parseFlags parses command line flags
func parseFlags() *FlagConfig {
    flags := &FlagConfig{}
    logErrors := flag.Bool("log-errors", true, "Enable error logging")

    // Define command line flags
    flag.StringVar(&flags.ConfigFile, "config", "",
        "Path to configuration file (JSON)")
    flag.StringVar(&flags.AccessLogPath, "access-log", "",
        "Path to Squid access log file")
    flag.StringVar(&flags.PositionFilePath, "position-file", "",
        "Path to file storing the last read position")
    flag.StringVar(&flags.OutputPath, "output", "",
        "Path where metrics will be written")
    flag.IntVar(&flags.BufferSize, "buffer-size", 0,
        "Buffer size for reading log file (in bytes)")
    flag.IntVar(&flags.RetryAttempts, "retry-attempts", 0,
        "Number of retry attempts for writing metrics")
    flag.StringVar(&flags.RetryDelay, "retry-delay", "",
        "Delay between retry attempts (e.g., '1s', '500ms')")
    flag.BoolVar(&flags.Version, "version", false,
        "Print version information")
    flag.StringVar(&flags.DomainsConfig, "domains-config", "",
        "Path to monitored domains YAML configuration file")

    flag.Parse()

    flags.LogErrors = logErrors
    return flags
}

// loadConfigFile loads configuration from a JSON file
func loadConfigFile(path string, config *Config) error {
    file, err := os.Open(path)
    if err != nil {
        return fmt.Errorf("failed to open config file: %v", err)
    }
    defer file.Close()

    decoder := json.NewDecoder(file)
    if err := decoder.Decode(config); err != nil {
        return fmt.Errorf("failed to parse config file: %v", err)
    }

    return nil
}

func validateConfig(config *Config) error {
    if config.AccessLogPath == "" {
        return fmt.Errorf("access log path is required")
    }
    if config.PositionFilePath == "" {
        return fmt.Errorf("position file path is required")
    }
    if config.OutputPath == "" {
        return fmt.Errorf("output path is required")
    }
    if config.BufferSize == 0 {
        config.BufferSize = 65536 // 64KB default buffer
    }
    if config.RetryAttempts == 0 {
        config.RetryAttempts = 3
    }
    if config.RetryDelay == "" {
        config.RetryDelay = "1s"
    }
    if config.LogFilePath == "" {
        return fmt.Errorf("log file path is required")
    }
    if config.KnownCodesFilePath == "" {
        return fmt.Errorf("known codes file path is required")
    }
    if config.KnownStatusFilePath == "" {
        config.KnownStatusFilePath = "/var/lib/squid_exporter/known_status.txt"
    }

    // Validate retry delay format
    if _, err := time.ParseDuration(config.RetryDelay); err != nil {
        return fmt.Errorf("invalid retry delay format: %v", err)
    }

    return nil
}

// loadMonitoredDomains loads the list of domains to monitor
func (mc *MetricsCollector) loadMonitoredDomains() error {
	if mc.config.MonitoredDomainsPath == "" {
		return nil // No domains to monitor
	}

	data, err := os.ReadFile(mc.config.MonitoredDomainsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read monitored domains file: %v", err)
	}

	var domainConfig DomainConfig
	if err := yaml.Unmarshal(data, &domainConfig); err != nil {
		return fmt.Errorf("failed to parse monitored domains file: %v", err)
	}

	mc.monitoredHosts = make(map[string]map[string]string)
	for _, target := range domainConfig.MonitoredTargets {
		mc.monitoredHosts[target.Host] = target.Labels
	}

	if mc.logger != nil {
		mc.logger.Printf("Loaded %d monitored targets", len(mc.monitoredHosts))
		for host, labels := range mc.monitoredHosts {
			if len(labels) > 0 {
				mc.logger.Printf("  - %s with labels: %v", host, labels)
			} else {
				mc.logger.Printf("  - %s (no labels)", host)
			}
		}
	}

	return nil
}
