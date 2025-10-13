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
	"fmt"
	"log"
	"sync"
	"time"
)

type Config struct {
	AccessLogPath         string `json:"access_log_path"`
	PositionFilePath      string `json:"position_file_path"`
	OutputPath            string `json:"output_path"`
	BufferSize            int    `json:"buffer_size"`
	LogErrors             bool   `json:"log_errors"`
	RetryAttempts         int    `json:"retry_attempts"`
	RetryDelay            string `json:"retry_delay"`
	LogFilePath           string `json:"log_file_path"`
	KnownCodesFilePath    string `json:"known_codes_file_path"`
	KnownStatusFilePath   string `json:"known_status_file_path"`
	MonitoredDomainsPath  string `json:"monitored_domains_path"`
}

// DomainConfig holds monitored domain configuration
type DomainConfig struct {
	MonitoredTargets []struct {
		Host   string            `yaml:"host"`
		Labels map[string]string `yaml:"labels,omitempty"`
	} `yaml:"monitored_targets"`
}

// DomainStats tracks statistics for a specific domain:port
type DomainStats struct {
	count         int64
	totalDuration float64
	maxDuration   float64
	minDuration   float64
	labels        map[string]string
	httpCodes     map[string]int64
}

// FlagConfig holds command line parameters
type FlagConfig struct {
	ConfigFile       string
	AccessLogPath    string
	PositionFilePath string
	OutputPath       string
	BufferSize       int
	LogErrors        *bool
	RetryAttempts    int
	RetryDelay       string
	Version          bool
	DomainsConfig    string
}

type MetricsCollector struct {
	config         Config
	mutex          sync.Mutex
	logger         *log.Logger
	retryDelay     time.Duration
	knownCodes     map[string]bool
	knownStatus    map[string]bool
	codesFile      string
	statusFile     string
	monitoredHosts map[string]map[string]string // host -> labels
	domainStats    map[string]*DomainStats
}

// Error types for better error handling
type (
	FileAccessError struct {
		Path string
		Err  error
	}

	ParseError struct {
		Line string
		Err  error
	}
)

func (e *FileAccessError) Error() string {
	return fmt.Sprintf("failed to access file %s: %v", e.Path, e.Err)
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("failed to parse line %s: %v", e.Line, e.Err)
}
