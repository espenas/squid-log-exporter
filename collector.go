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
    "os"
    "path/filepath"
    "sort"
    "strings"
    "time"
)

func NewMetricsCollector(config Config) (*MetricsCollector, error) {
    // Validate configuration
    if err := validateConfig(&config); err != nil {
        return nil, fmt.Errorf("invalid configuration: %v", err)
    }

    retryDelay, err := time.ParseDuration(config.RetryDelay)
    if err != nil {
        return nil, fmt.Errorf("invalid retry delay format: %v", err)
    }

    // Setup logger
    var logger *log.Logger
    if config.LogErrors {
        logFile, err := os.OpenFile(
            config.LogFilePath,
            os.O_APPEND|os.O_CREATE|os.O_WRONLY,
            0644,
        )
        if err != nil {
            return nil, fmt.Errorf("failed to create log file: %v", err)
        }
        logger = log.New(logFile, "", log.LstdFlags)
    }

    mc := &MetricsCollector{
        config:      config,
        logger:      logger,
        retryDelay:  retryDelay,
        knownStatus: make(map[string]bool),
        statusFile:  config.KnownStatusFilePath,
        domainStats: make(map[string]*DomainStats),
    }

    // Load known status
    if err := mc.loadKnownStatus(); err != nil {
        return nil, fmt.Errorf("failed to load known status: %v", err)
    }

    // Load monitored domains
    if err := mc.loadMonitoredDomains(); err != nil {
        return nil, fmt.Errorf("failed to load monitored domains: %v", err)
    }

    return mc, nil
}

func (mc *MetricsCollector) logError(err error) {
    if mc.logger != nil {
        mc.logger.Printf("ERROR: %v", err)
    }
}

func (mc *MetricsCollector) readLastPosition() (int64, uint64, error) {
    file, err := os.Open(mc.config.PositionFilePath)
    if err != nil {
        if os.IsNotExist(err) {
            return 0, 0, nil
        }
        return 0, 0, &FileAccessError{Path: mc.config.PositionFilePath, Err: err}
    }
    defer file.Close()

    var position int64
    var inode uint64
    if _, err := fmt.Fscanf(file, "%d %d", &position, &inode); err != nil {
        return 0, 0, fmt.Errorf("failed to parse position file: %v", err)
    }
    return position, inode, nil
}

func (mc *MetricsCollector) writeLastPosition(position int64, inode uint64) error {
    mc.mutex.Lock()
    defer mc.mutex.Unlock()

    // Create directory if it doesn't exist
    if err := os.MkdirAll(filepath.Dir(mc.config.PositionFilePath), 0755); err != nil {
        return &FileAccessError{Path: mc.config.PositionFilePath, Err: err}
    }

    file, err := os.Create(mc.config.PositionFilePath)
    if err != nil {
        return &FileAccessError{Path: mc.config.PositionFilePath, Err: err}
    }
    defer file.Close()

    if _, err := fmt.Fprintf(file, "%d %d", position, inode); err != nil {
        return fmt.Errorf("failed to write position: %v", err)
    }
    return nil
}

func (mc *MetricsCollector) loadKnownStatus() error {
    mc.mutex.Lock()
    defer mc.mutex.Unlock()

    data, err := os.ReadFile(mc.statusFile)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return fmt.Errorf("failed to read known status file: %v", err)
    }

    for _, status := range strings.Split(string(data), "\n") {
        status = strings.TrimSpace(status)
        if status != "" {
            mc.knownStatus[status] = true
        }
    }

    return nil
}

func (mc *MetricsCollector) saveKnownStatus() error {
    mc.mutex.Lock()
    defer mc.mutex.Unlock()

    if err := os.MkdirAll(filepath.Dir(mc.statusFile), 0755); err != nil {
        return fmt.Errorf("failed to create directory: %v", err)
    }

    var status []string
    for s := range mc.knownStatus {
        status = append(status, s)
    }
    sort.Strings(status)

    tmpfile, err := os.CreateTemp(filepath.Dir(mc.statusFile), "known_status.*")
    if err != nil {
        return fmt.Errorf("failed to create temp file: %v", err)
    }
    tmpName := tmpfile.Name()
    defer os.Remove(tmpName)

    for _, s := range status {
        if _, err := fmt.Fprintln(tmpfile, s); err != nil {
            tmpfile.Close()
            return fmt.Errorf("failed to write status: %v", err)
        }
    }

    if err := tmpfile.Close(); err != nil {
        return fmt.Errorf("failed to close temp file: %v", err)
    }

    if err := os.Rename(tmpName, mc.statusFile); err != nil {
        return fmt.Errorf("failed to save known status: %v", err)
    }

    return nil
}
