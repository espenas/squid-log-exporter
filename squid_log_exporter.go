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
    "bufio"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "log"
    "os"
    "path/filepath"
    "regexp"
    "sort"
    "strconv"
    "strings"
    "sync"
    "syscall"
    "time"
)

type Config struct {
    AccessLogPath       string `json:"access_log_path"`
    PositionFilePath    string `json:"position_file_path"`
    OutputPath          string `json:"output_path"`
    BufferSize          int    `json:"buffer_size"`
    LogErrors           bool   `json:"log_errors"`
    RetryAttempts       int    `json:"retry_attempts"`
    RetryDelay          string `json:"retry_delay"`
    LogFilePath         string `json:"log_file_path"`
    KnownCodesFilePath  string `json:"known_codes_file_path"`
    KnownStatusFilePath string `json:"known_status_file_path"`
}

// FlagConfig holds command line parameters
type FlagConfig struct {
    ConfigFile       string
    AccessLogPath    string
    PositionFilePath string
    OutputPath       string
    BufferSize       int
    LogErrors        *bool  // Using pointer to detect if flag was set
    RetryAttempts    int
    RetryDelay       string
    Version          bool
}

type MetricsCollector struct {
    config      Config
    mutex       sync.Mutex
    logger      *log.Logger
    retryDelay  time.Duration
    knownCodes  map[string]bool    // Track known HTTP codes
    knownStatus map[string]bool
    codesFile   string            // Path to store known codes
    statusFile  string
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

func getFileInode(filepath string) (uint64, error) {
    fileInfo, err := os.Stat(filepath)
    if err != nil {
        return 0, err
    }
    stat, ok := fileInfo.Sys().(*syscall.Stat_t)
    if !ok {
        return 0, fmt.Errorf("failed to get file inode")
    }
    return stat.Ino, nil
}

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
        knownCodes:  make(map[string]bool),
        knownStatus: make(map[string]bool),
        codesFile:   config.KnownCodesFilePath,
        statusFile:  config.KnownStatusFilePath,
    }

    // Load known codes and status
    if err := mc.loadKnownCodes(); err != nil {
        return nil, fmt.Errorf("failed to load known codes: %v", err)
    }
    if err := mc.loadKnownStatus(); err != nil {
        return nil, fmt.Errorf("failed to load known status: %v", err)
    }

    return mc, nil
}

// Function to load known status
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

func (mc *MetricsCollector) loadKnownCodes() error {
    mc.mutex.Lock()
    defer mc.mutex.Unlock()

    // If file doesn't exist, that's ok - we'll create it when needed
    data, err := os.ReadFile(mc.codesFile)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return fmt.Errorf("failed to read known codes file: %v", err)
    }

    // Parse codes from file
    for _, code := range strings.Split(string(data), "\n") {
        code = strings.TrimSpace(code)
        if code != "" {
            mc.knownCodes[code] = true
        }
    }

    return nil
}

// Function to save known status
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

func (mc *MetricsCollector) saveKnownCodes() error {
    mc.mutex.Lock()
    defer mc.mutex.Unlock()

    // Create directory if it doesn't exist
    if err := os.MkdirAll(filepath.Dir(mc.codesFile), 0755); err != nil {
        return fmt.Errorf("failed to create directory: %v", err)
    }

    // Collect and sort codes for consistent file content
    var codes []string
    for code := range mc.knownCodes {
        codes = append(codes, code)
    }
    sort.Strings(codes)

    // Write to temporary file first
    tmpfile, err := os.CreateTemp(filepath.Dir(mc.codesFile), "known_codes.*")
    if err != nil {
        return fmt.Errorf("failed to create temp file: %v", err)
    }
    tmpName := tmpfile.Name()
    defer os.Remove(tmpName)

    // Write codes to file
    for _, code := range codes {
        if _, err := fmt.Fprintln(tmpfile, code); err != nil {
            tmpfile.Close()
            return fmt.Errorf("failed to write code: %v", err)
        }
    }

    if err := tmpfile.Close(); err != nil {
        return fmt.Errorf("failed to close temp file: %v", err)
    }

    // Atomic rename
    if err := os.Rename(tmpName, mc.codesFile); err != nil {
        return fmt.Errorf("failed to save known codes: %v", err)
    }

    return nil
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

func (mc *MetricsCollector) parseNewEntries(lastPosition int64, lastInode uint64) (map[string]int, map[string]int, int, map[string]map[string]int, error) {
    file, err := os.Open(mc.config.AccessLogPath)
    if err != nil {
        return nil, nil, 0, nil, &FileAccessError{Path: mc.config.AccessLogPath, Err: err}
    }
    defer file.Close()

    // Get current file information
    currentInode, err := getFileInode(mc.config.AccessLogPath)
    if err != nil {
        return nil, nil, 0, nil, err
    }

    // Get file size
    fileInfo, err := file.Stat()
    if err != nil {
        return nil, nil, 0, nil, fmt.Errorf("failed to get file info: %v", err)
    }
    fileSize := fileInfo.Size()

    // Handle file rotation cases
    if lastInode != currentInode {
        lastPosition = 0
    } else if lastPosition > fileSize {
        lastPosition = 0
    }

    // Seek to last position
    if lastPosition > 0 {
        if _, err := file.Seek(lastPosition, io.SeekStart); err != nil {
            return nil, nil, 0, nil, fmt.Errorf("failed to seek to position %d: %v", lastPosition, err)
        }
    }

    codeCounts := make(map[string]int)
    cacheCounts := make(map[string]int)
    durationCounts := map[string]map[string]int{
        "ms": make(map[string]int),
        "s":  make(map[string]int),
    }
    totalConnections := 0
    scanner := bufio.NewScanner(file)
    scanner.Buffer(make([]byte, mc.config.BufferSize), mc.config.BufferSize)

    // Updated regex patterns
    cacheRegex := regexp.MustCompile(`\b(TCP_(?:HIT|MISS|DENIED|TUNNEL))\b`)

    var bytesRead int64
    if lastPosition > 0 {
        bytesRead = lastPosition
    }

    totalDurationNonTunnel := 0.0
    totalConnectionsNonTunnel := 0

    for scanner.Scan() {
        line := scanner.Text()
        bytesRead += int64(len(line)) + 1 // +1 for newline

        fields := strings.Fields(line)
        if len(fields) >= 4 {
            totalConnections++

            // Extract cache status first
            statusField := fields[3]
            parts := strings.Split(statusField, "/")

            if len(parts) > 0 {
                matches := cacheRegex.FindStringSubmatch(parts[0])
                if len(matches) > 1 {
                    cacheStatus := matches[1]
                    cacheCounts[cacheStatus]++
                    
                    // Track new status
                    if !mc.knownStatus[cacheStatus] {
                        mc.knownStatus[cacheStatus] = true
                        if err := mc.saveKnownStatus(); err != nil {
                            mc.logError(fmt.Errorf("failed to save new cache status: %v", err))
                        }
                    }
                }
            }

            // Only process duration for non-TUNNEL connections
            if matches := cacheRegex.FindStringSubmatch(parts[0]); len(matches) > 1 && matches[1] != "TCP_TUNNEL" && len(fields) >= 2 {
                if duration, err := strconv.ParseFloat(fields[1], 64); err == nil {
                    totalDurationNonTunnel += duration
                    totalConnectionsNonTunnel++

                    // Duration in milliseconds
                    switch {
                    case duration <= 200:
                        durationCounts["ms"]["0-200"]++
                    case duration <= 400:
                        durationCounts["ms"]["200-400"]++
                    case duration <= 600:
                        durationCounts["ms"]["400-600"]++
                    case duration <= 800:
                        durationCounts["ms"]["600-800"]++
                    case duration <= 1000:
                        durationCounts["ms"]["800-1000"]++
                    default:
                        durationCounts["ms"]["over1000"]++
                    }

                    // Duration in seconds
                    durationSec := duration / 1000.0
                    switch {
                    case durationSec <= 1.0:
                        durationCounts["s"]["0-1"]++
                    case durationSec <= 2.0:
                        durationCounts["s"]["1-2"]++
                    case durationSec <= 3.0:
                        durationCounts["s"]["2-3"]++
                    case durationSec <= 4.0:
                        durationCounts["s"]["3-4"]++
                    case durationSec <= 5.0:
                        durationCounts["s"]["4-5"]++
                    default:
                        durationCounts["s"]["over5"]++
                    }
                }
            }

            // Process HTTP status codes
            if len(parts) > 1 {
                httpCode := parts[1]
                if matched, _ := regexp.MatchString(`^\d{3}$`, httpCode); matched {
                    codeCounts[httpCode]++
                    if !mc.knownCodes[httpCode] {
                        mc.knownCodes[httpCode] = true
                        if err := mc.saveKnownCodes(); err != nil {
                            mc.logError(fmt.Errorf("failed to save new HTTP code: %v", err))
                        }
                    }
                }
            }
        }
    }

    if err := scanner.Err(); err != nil {
        return nil, nil, 0, nil, fmt.Errorf("error scanning file: %v", err)
    }

    // Only update position if we actually read something
    if bytesRead > lastPosition {
        if err := mc.writeLastPosition(bytesRead, currentInode); err != nil {
            return nil, nil, 0, nil, err
        }
    }

    // Log some statistics if error logging is enabled
    if mc.logger != nil {
        avgDuration := 0.0
        if totalConnectionsNonTunnel > 0 {
            avgDuration = totalDurationNonTunnel / float64(totalConnectionsNonTunnel)
        }
        mc.logger.Printf(
            "Processed %d total connections (%d non-tunnel). Average duration for non-tunnel requests: %.2fms",
            totalConnections,
            totalConnectionsNonTunnel,
            avgDuration,
        )
    }

    return codeCounts, cacheCounts, totalConnections, durationCounts, nil
}

func (mc *MetricsCollector) writeMetrics(codeCounts map[string]int, cacheCounts map[string]int, totalConnections int, durationCounts map[string]map[string]int) error {
    mc.mutex.Lock()
    defer mc.mutex.Unlock()

    // Ensure directory exists
    if err := os.MkdirAll(filepath.Dir(mc.config.OutputPath), 0755); err != nil {
        return &FileAccessError{Path: mc.config.OutputPath, Err: err}
    }

    tmpfile, err := os.CreateTemp(filepath.Dir(mc.config.OutputPath), "metrics.*")
    if err != nil {
        return fmt.Errorf("failed to create temp file: %v", err)
    }
    tmpName := tmpfile.Name()

    defer func() {
        tmpfile.Close()
        os.Remove(tmpName)
    }()

    // Write total connections metric
    if _, err := fmt.Fprintf(tmpfile, "# HELP squid_connections_total Total number of connections\n"); err != nil {
        return fmt.Errorf("failed to write connections help: %v", err)
    }
    if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_connections_total counter\n"); err != nil {
        return fmt.Errorf("failed to write connections type: %v", err)
    }
    if _, err := fmt.Fprintf(tmpfile, "squid_connections_total %d\n\n", totalConnections); err != nil {
        return fmt.Errorf("failed to write connections metric: %v", err)
    }

    // Write millisecond duration metrics
    if _, err := fmt.Fprintf(tmpfile, "# HELP squid_request_duration_milliseconds_total Number of requests by duration interval in milliseconds\n"); err != nil {
        return fmt.Errorf("failed to write duration help: %v", err)
    }
    if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_request_duration_milliseconds_total counter\n"); err != nil {
        return fmt.Errorf("failed to write duration type: %v", err)
    }

    // Define millisecond duration intervals in order
    msIntervals := []string{"0-200", "200-400", "400-600", "600-800", "800-1000", "over1000"}
    for _, interval := range msIntervals {
        count := durationCounts["ms"][interval]
        if _, err := fmt.Fprintf(tmpfile, "squid_request_duration_milliseconds_total{interval=\"%s\"} %d\n",
            interval, count); err != nil {
            return fmt.Errorf("failed to write duration metrics: %v", err)
        }
    }
    if _, err := fmt.Fprintln(tmpfile); err != nil {
        return fmt.Errorf("failed to write separator: %v", err)
    }

    // Write second duration metrics
    if _, err := fmt.Fprintf(tmpfile, "# HELP squid_request_duration_seconds_total Number of requests by duration interval in seconds\n"); err != nil {
        return fmt.Errorf("failed to write duration help: %v", err)
    }
    if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_request_duration_seconds_total counter\n"); err != nil {
        return fmt.Errorf("failed to write duration type: %v", err)
    }

    // Define second duration intervals in order
    sIntervals := []string{"0-1", "1-2", "2-3", "3-4", "4-5", "over5"}
    for _, interval := range sIntervals {
        count := durationCounts["s"][interval]
        if _, err := fmt.Fprintf(tmpfile, "squid_request_duration_seconds_total{interval=\"%s\"} %d\n",
            interval, count); err != nil {
            return fmt.Errorf("failed to write duration metrics: %v", err)
        }
    }
    if _, err := fmt.Fprintln(tmpfile); err != nil {
        return fmt.Errorf("failed to write separator: %v", err)
    }

    // Write HTTP response metrics
    if _, err := fmt.Fprintf(tmpfile, "# HELP squid_http_responses_total Total number of HTTP responses by status code and category\n"); err != nil {
        return fmt.Errorf("failed to write metric help: %v", err)
    }
    if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_http_responses_total counter\n"); err != nil {
        return fmt.Errorf("failed to write metric type: %v", err)
    }

    // Track categories for all codes
    categories := make(map[string]int)

    // Write metrics for all known HTTP codes and track their categories
    for code := range mc.knownCodes {
        count := codeCounts[code]
        category := code[:1] + "xx"
        categories[category] += count

        if _, err := fmt.Fprintf(tmpfile, "squid_http_responses_total{code=\"%s\",category=\"%s\"} %d\n",
            code, category, count); err != nil {
            return fmt.Errorf("failed to write metrics: %v", err)
        }
    }

    // Get sorted keys for consistent output
    codes := make([]string, 0, len(codeCounts))
    for code := range codeCounts {
        codes = append(codes, code)
    }
    sort.Strings(codes)

    // Add any new codes that weren't in known codes
    for _, code := range codes {
        if !mc.knownCodes[code] {
            category := code[:1] + "xx"
            categories[category] += codeCounts[code]

            if _, err := fmt.Fprintf(tmpfile, "squid_http_responses_total{code=\"%s\",category=\"%s\"} %d\n",
                code, category, codeCounts[code]); err != nil {
                return fmt.Errorf("failed to write metrics: %v", err)
            }
        }
    }

    // Add a blank line before category totals
    if _, err := fmt.Fprintln(tmpfile); err != nil {
        return fmt.Errorf("failed to write separator: %v", err)
    }

    // Write help and type for category totals
    if _, err := fmt.Fprintf(tmpfile, "# HELP squid_http_responses_by_category_total Total number of HTTP responses by status code category\n"); err != nil {
        return fmt.Errorf("failed to write category help: %v", err)
    }
    if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_http_responses_by_category_total counter\n"); err != nil {
        return fmt.Errorf("failed to write category type: %v", err)
    }

    // Write category totals
    categoryNames := make([]string, 0, len(categories))
    for category := range categories {
        categoryNames = append(categoryNames, category)
    }
    sort.Strings(categoryNames)

    for _, category := range categoryNames {
        if _, err := fmt.Fprintf(tmpfile, "squid_http_responses_by_category_total{category=\"%s\"} %d\n",
            category, categories[category]); err != nil {
            return fmt.Errorf("failed to write category metrics: %v", err)
        }
    }

    // Add a blank line before cache metrics
    if _, err := fmt.Fprintln(tmpfile); err != nil {
        return fmt.Errorf("failed to write separator: %v", err)
    }

    // Write cache status metrics
    if _, err := fmt.Fprintf(tmpfile, "# HELP squid_cache_status_total Total number of requests by cache status\n"); err != nil {
        return fmt.Errorf("failed to write cache help: %v", err)
    }
    if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_cache_status_total counter\n"); err != nil {
        return fmt.Errorf("failed to write cache type: %v", err)
    }

    // Get sorted status for consistent output
    var allStatus []string
    for status := range mc.knownStatus {
        allStatus = append(allStatus, status)
    }
    sort.Strings(allStatus)

    // Write all known status, defaulting to 0 if not found in counts
    for _, status := range allStatus {
        count := cacheCounts[status] // Will be 0 if status not found
        if _, err := fmt.Fprintf(tmpfile, "squid_cache_status_total{status=\"%s\"} %d\n",
            status, count); err != nil {
            return fmt.Errorf("failed to write cache metrics: %v", err)
        }
    }

    if err := tmpfile.Sync(); err != nil {
        return fmt.Errorf("failed to sync temporary file: %v", err)
    }

    if err := tmpfile.Close(); err != nil {
        return fmt.Errorf("failed to close temporary file: %v", err)
    }

    // Atomic rename
    if err := os.Rename(tmpName, mc.config.OutputPath); err != nil {
        return &FileAccessError{Path: mc.config.OutputPath, Err: err}
    }

    // Set file permissions to 0644
    if err := os.Chmod(mc.config.OutputPath, 0644); err != nil {
        return fmt.Errorf("failed to set file permissions: %v", err)
    }

    return nil
}

func (mc *MetricsCollector) writeMetricsWithRetry(codeCounts map[string]int, cacheCounts map[string]int, totalConnections int, durationCounts map[string]map[string]int) error {
    var lastErr error
    for attempt := 0; attempt < mc.config.RetryAttempts; attempt++ {
        if err := mc.writeMetrics(codeCounts, cacheCounts, totalConnections, durationCounts); err != nil {
            lastErr = err
            mc.logError(fmt.Errorf("attempt %d failed: %v", attempt+1, err))
            time.Sleep(mc.retryDelay)
            continue
        }
        return nil
    }
    return fmt.Errorf("failed after %d attempts, last error: %v", mc.config.RetryAttempts, lastErr)
}

// loadConfig loads configuration from file and command line flags
func loadConfig() (*Config, error) {
    // Define default configuration
    config := &Config{
        AccessLogPath:      "/var/log/squid/access.log",
        PositionFilePath:   "/var/lib/squid_exporter/position.txt",
        OutputPath:         "/var/lib/squid_exporter/metrics.txt",
        BufferSize:         65536,
        LogErrors:          true,
        RetryAttempts:      3,
        RetryDelay:         "1s",
        LogFilePath:        "/var/lib/squid_exporter/squid_metrics.log",
        KnownCodesFilePath: "/var/lib/squid_exporter/known_http_codes.txt",
    }

    // Parse command line flags
    flags := parseFlags()

    // If version flag is set, print version and exit
    if flags.Version {
        fmt.Println("Squid Log Exporter v1.0.0")
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
    if flags.LogErrors != nil {  // Check if the flag was set
        config.LogErrors = *flags.LogErrors
    }
    if flags.RetryAttempts != 0 {
        config.RetryAttempts = flags.RetryAttempts
    }
    if flags.RetryDelay != "" {
        config.RetryDelay = flags.RetryDelay
    }

    return config, nil
}

// parseFlags parses command line flags
func parseFlags() *FlagConfig {
    flags := &FlagConfig{}
    logErrors := flag.Bool("log-errors", true, "Enable error logging")  // Create bool pointer

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

    flag.Parse()

    flags.LogErrors = logErrors  // Store the pointer to the bool
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
        return fmt.Errorf("known status file path is required")
    }

    // Validate retry delay format
    if _, err := time.ParseDuration(config.RetryDelay); err != nil {
        return fmt.Errorf("invalid retry delay format: %v", err)
    }

    return nil
}

func main() {
    // Load configuration
    config, err := loadConfig()
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    // Validate configuration after loading
    if err := validateConfig(config); err != nil {
        log.Fatalf("Invalid configuration: %v", err)
    }

    collector, err := NewMetricsCollector(*config)
    if err != nil {
        log.Fatalf("Failed to initialize metrics collector: %v", err)
    }

    lastPosition, lastInode, err := collector.readLastPosition()
    if err != nil {
        log.Fatalf("Failed to read last position: %v", err)
    }

    codeCounts, cacheCounts, totalConnections, durationCounts, err := collector.parseNewEntries(lastPosition, lastInode)
    if err != nil {
        log.Fatalf("Failed to parse log entries: %v", err)
    }

    // Pass durationCounts directly to writeMetricsWithRetry since the types now match
    if err := collector.writeMetricsWithRetry(codeCounts, cacheCounts, totalConnections, durationCounts); err != nil {
        log.Fatalf("Failed to write metrics: %v", err)
    }

    fmt.Printf("Successfully processed %d connections with %d HTTP status codes and %d cache statuses\n",
        totalConnections, len(codeCounts), len(cacheCounts))
}
