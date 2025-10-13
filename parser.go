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
    "fmt"
    "io"
    "net/url"
    "os"
    "regexp"
    "strconv"
    "strings"
    "syscall"
)

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

// extractHostPort extracts host:port from a Squid request URL
func extractHostPort(requestURL string) string {
    // For CONNECT requests: "test.example.com:443"
    if !strings.Contains(requestURL, "://") {
        // Already in format host:port or just host
        if strings.Contains(requestURL, ":") {
            // Remove any path
            if idx := strings.Index(requestURL, "/"); idx != -1 {
                requestURL = requestURL[:idx]
            }
            return requestURL
        }
        // If just hostname without port, assume 80
        return requestURL + ":80"
    }

    // For HTTP/HTTPS requests
    u, err := url.Parse(requestURL)
    if err != nil {
        return ""
    }

    host := u.Hostname()
    port := u.Port()

    if port == "" {
        if u.Scheme == "https" {
            port = "443"
        } else {
            port = "80"
        }
    }

    return host + ":" + port
}

// trackDomainRequest tracks statistics for a monitored domain
func (mc *MetricsCollector) trackDomainRequest(hostPort string, duration float64) {
    if len(mc.monitoredHosts) == 0 || !mc.monitoredHosts[hostPort] {
        return
    }

    stats, exists := mc.domainStats[hostPort]
    if !exists {
        stats = &DomainStats{
            minDuration: duration,
            maxDuration: duration,
        }
        mc.domainStats[hostPort] = stats
    }

    stats.count++
    stats.totalDuration += duration

    if duration > stats.maxDuration {
        stats.maxDuration = duration
    }
    if duration < stats.minDuration {
        stats.minDuration = duration
    }
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

            // Parse duration for all connections (including TCP_TUNNEL for domain tracking)
            var duration float64
            if len(fields) >= 2 {
                if durationMs, err := strconv.ParseFloat(fields[1], 64); err == nil {
                    duration = durationMs
                    durationSeconds := durationMs / 1000.0

                    // Track domain statistics (includes TCP_TUNNEL)
                    if len(fields) >= 7 {
                        requestURL := fields[6]
                        hostPort := extractHostPort(requestURL)
                        if hostPort != "" {
                            mc.trackDomainRequest(hostPort, durationSeconds)
                        }
                    }

                    // Only process duration buckets for non-TUNNEL connections
                    if matches := cacheRegex.FindStringSubmatch(parts[0]); len(matches) > 1 && matches[1] != "TCP_TUNNEL" {
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

        // Log domain statistics
        if len(mc.domainStats) > 0 {
            mc.logger.Printf("Domain statistics:")
            for hostPort, stats := range mc.domainStats {
                avgDuration := stats.totalDuration / float64(stats.count)
                mc.logger.Printf("  %s: count=%d, avg=%.2fs, min=%.2fs, max=%.2fs",
                    hostPort, stats.count, avgDuration, stats.minDuration, stats.maxDuration)
            }
        }
    }

    return codeCounts, cacheCounts, totalConnections, durationCounts, nil
}
