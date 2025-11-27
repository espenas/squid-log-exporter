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

package parser

import (
        "bufio"
        "fmt"
        "io"
        "log"
        "net/url"
        "os"
        "sort"
        "strconv"
        "strings"
        "sync"

        "squid-log-exporter/internal/config"
        "squid-log-exporter/internal/metrics"
        "squid-log-exporter/internal/position"
)

// Parser handles parsing of Squid access logs
type Parser struct {
        metrics         *metrics.Metrics
        config          *config.Config
        positionTracker *position.Tracker
        logFile         string
        allDomainsSeen  map[string]bool
        mu              sync.RWMutex
}

// DomainData holds statistics for a domain
type DomainData struct {
        Requests             int
        BytesIn              int64
        BytesOut             int64
        ResponsesByCode      map[string]map[string]int // code -> category -> count
        ResponsesByCategory  map[string]int
        Durations            []float64
        CacheHits            int
        CacheMisses          int
}

// NewParser creates a new parser instance
func NewParser(logFile string, positionFile string, m *metrics.Metrics, cfg *config.Config) *Parser {
        return &Parser{
                metrics:         m,
                config:          cfg,
                positionTracker: position.NewTracker(positionFile),
                logFile:         logFile,
                allDomainsSeen:  make(map[string]bool),
        }
}

// Parse reads and processes the Squid log file
func (p *Parser) Parse() error {
        // Load last position
        if err := p.positionTracker.Load(); err != nil {
                log.Printf("Warning: failed to load position: %v, starting from beginning", err)
        }

        file, err := os.Open(p.logFile)
        if err != nil {
                return fmt.Errorf("failed to open log file: %w", err)
        }
        defer file.Close()

        // Get current file inode
        currentInode, err := position.GetFileInode(p.logFile)
        if err != nil {
                return fmt.Errorf("failed to get file inode: %w", err)
        }

        lastPos, lastInode := p.positionTracker.GetPosition()

        log.Printf("Debug: lastPos=%d, lastInode=%d, currentInode=%d", lastPos, lastInode, currentInode) // DEBUG

        // Check for log rotation
        if currentInode != lastInode && lastInode != 0 {
                log.Printf("Log rotation detected (inode changed: %d -> %d), starting from beginning", lastInode, currentInode)
                lastPos = 0
        }

        // Seek to last position
        if lastPos > 0 {
                log.Printf("Seeking to position %d", lastPos) // DEBUG
                if _, err := file.Seek(lastPos, io.SeekStart); err != nil {
                        log.Printf("Warning: failed to seek to position %d: %v, starting from beginning", lastPos, err)
                        lastPos = 0
                        file.Seek(0, io.SeekStart)
                }
        } else {
                log.Printf("Starting from beginning (lastPos=0)") // DEBUG
        }

        // Statistics
        stats := &Stats{
                Connections:      0,
                RequestDurations: make(map[string]int),
                CacheStatuses:    make(map[string]int),
                HTTPResponses:    make(map[string]map[string]int),
                HTTPByCategory:   make(map[string]int),
                DomainData:       make(map[string]map[string]*DomainData),
        }

        scanner := bufio.NewScanner(file)
        const maxScanTokenSize = 1024 * 1024
        buf := make([]byte, maxScanTokenSize)
        scanner.Buffer(buf, maxScanTokenSize)

        lineCount := 0

        for scanner.Scan() {
                line := scanner.Text()

                if err := p.parseLine(line, stats); err != nil {
                        log.Printf("Warning: failed to parse line: %v", err)
                        continue
                }

                lineCount++

                // Save position every 1000 lines
                if lineCount%1000 == 0 {
                        currentPos, _ := file.Seek(0, io.SeekCurrent)
                        if err := p.positionTracker.Save(p.logFile, currentPos, currentInode); err != nil {
                                log.Printf("Warning: failed to save position: %v", err)
                        }
                }
        }

        if err := scanner.Err(); err != nil {
                return fmt.Errorf("scanner error: %w", err)
        }

        // Update metrics
        p.updateMetrics(stats)

        // Save final position
        finalPos, _ := file.Seek(0, io.SeekCurrent)
        log.Printf("Saving final position: %d (parsed %d lines)", finalPos, lineCount) // DEBUG
        if err := p.positionTracker.Save(p.logFile, finalPos, currentInode); err != nil {
                log.Printf("Warning: failed to save final position: %v", err)
        }

        log.Printf("Parsed %d new lines", lineCount)

        return nil
}

// Stats holds all parsed statistics
type Stats struct {
        Connections      int
        RequestDurations map[string]int
        CacheStatuses    map[string]int
        HTTPResponses    map[string]map[string]int
        HTTPByCategory   map[string]int
        DomainData       map[string]map[string]*DomainData // host -> port -> data
}

// parseLine parses a single Squid access log line using configured format
func (p *Parser) parseLine(line string, stats *Stats) error {
        // Split by spaces, but preserve quoted strings
        fields := splitPreservingQuotes(line)

        // Get minimum required fields
        minFields := 0
        for _, idx := range p.config.LogFormat.Fields {
                if idx > minFields {
                        minFields = idx
                }
        }

        if len(fields) <= minFields {
                return fmt.Errorf("invalid log line format: expected at least %d fields, got %d", minFields+1, len(fields))
        }

        // Extract fields using configured positions
        elapsed := p.config.LogFormat.GetField(fields, "duration")
        resultCode := p.config.LogFormat.GetField(fields, "result_code")
        bytes := p.config.LogFormat.GetField(fields, "bytes")
        method := p.config.LogFormat.GetField(fields, "method")
        urlStr := p.config.LogFormat.GetField(fields, "url")

        // Parse result code (e.g., "TCP_TUNNEL/200")
        parts := strings.Split(resultCode, "/")
        if len(parts) != 2 {
                return fmt.Errorf("invalid result code format: %s", resultCode)
        }

        cacheStatus := parts[0]
        httpCode := parts[1]
        category := categorizeHTTPCode(httpCode)

        // Parse duration
        duration, err := strconv.ParseFloat(elapsed, 64)
        if err != nil {
                duration = 0
        }

        // Convert to seconds based on config
        var durationSeconds float64
        if p.config.LogFormat.DurationUnit == "ms" {
                durationSeconds = duration / 1000.0
        } else {
                durationSeconds = duration
        }

        // Parse bytes
        bytesInt, err := strconv.ParseInt(bytes, 10, 64)
        if err != nil {
                bytesInt = 0
        }

        // Update global stats
        stats.Connections++
        durationBucket := getDurationBucket(durationSeconds)
        stats.RequestDurations[durationBucket]++
        stats.CacheStatuses[cacheStatus]++

        if stats.HTTPResponses[httpCode] == nil {
                stats.HTTPResponses[httpCode] = make(map[string]int)
        }
        stats.HTTPResponses[httpCode][category]++
        stats.HTTPByCategory[category]++

        // Skip internal Squid URLs
        if strings.HasPrefix(urlStr, "cache_object://") ||
                strings.HasPrefix(urlStr, "mgr://") ||
                strings.HasPrefix(urlStr, "internal://") ||
                strings.HasPrefix(urlStr, "urn:") {
                return nil
        }

        // Parse host and port from URL
        var host, port string

        // Handle CONNECT method (format: host:port)
        if method == "CONNECT" {
                hostPort := strings.Split(urlStr, ":")
                if len(hostPort) >= 1 {
                        host = hostPort[0]
                        if len(hostPort) >= 2 {
                                port = hostPort[1]
                        } else {
                                port = "443"
                        }
                }
        } else {
                // Handle regular HTTP/HTTPS URLs
                if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
                        parsedURL, err := url.Parse(urlStr)
                        if err != nil {
                                return nil
                        }
                        host = parsedURL.Hostname()
                        port = parsedURL.Port()
                        if port == "" {
                                if parsedURL.Scheme == "https" {
                                        port = "443"
                                } else {
                                        port = "80"
                                }
                        }
                } else {
                        // Might be just "host:port" format
                        hostPort := strings.Split(urlStr, ":")
                        if len(hostPort) >= 1 {
                                host = hostPort[0]
                                if len(hostPort) >= 2 {
                                        port = hostPort[1]
                                } else {
                                        port = "80"
                                }
                        }
                }
        }

        // Skip if no valid host
        if host == "" || host == "-" || host == "localhost" {
                return nil
        }

        // Domain-specific stats
        p.updateDomainStats(stats, host, port, bytesInt, httpCode, category, durationSeconds, cacheStatus)

        return nil
}

// splitPreservingQuotes splits a string by spaces but preserves quoted strings
func splitPreservingQuotes(s string) []string {
        var fields []string
        var current strings.Builder
        inQuote := false

        for i := 0; i < len(s); i++ {
                char := s[i]

                if char == '"' {
                        inQuote = !inQuote
                        continue
                }

                if char == ' ' && !inQuote {
                        if current.Len() > 0 {
                                fields = append(fields, current.String())
                                current.Reset()
                        }
                        continue
                }

                current.WriteByte(char)
        }

        if current.Len() > 0 {
                fields = append(fields, current.String())
        }

        return fields
}

func (p *Parser) updateDomainStats(stats *Stats, host, port string, bytes int64, httpCode, category string, duration float64, cacheStatus string) {
        if stats.DomainData[host] == nil {
                stats.DomainData[host] = make(map[string]*DomainData)
        }

        if stats.DomainData[host][port] == nil {
                stats.DomainData[host][port] = &DomainData{
                        ResponsesByCode:     make(map[string]map[string]int),
                        ResponsesByCategory: make(map[string]int),
                        Durations:           []float64{},
                }
        }

        data := stats.DomainData[host][port]
        data.Requests++
        data.BytesOut += bytes
        data.Durations = append(data.Durations, duration)

        // HTTP responses
        if data.ResponsesByCode[httpCode] == nil {
                data.ResponsesByCode[httpCode] = make(map[string]int)
        }
        data.ResponsesByCode[httpCode][category]++
        data.ResponsesByCategory[category]++

        // Cache stats
        if strings.HasPrefix(cacheStatus, "TCP_HIT") || strings.HasPrefix(cacheStatus, "TCP_MEM_HIT") {
                data.CacheHits++
        } else if strings.HasPrefix(cacheStatus, "TCP_MISS") {
                data.CacheMisses++
        }
}

func (p *Parser) updateMetrics(stats *Stats) {
	// Global metrics
	p.metrics.SetConnections(stats.Connections)

	for interval, count := range stats.RequestDurations {
		p.metrics.SetRequestDuration(interval, count)
	}

	for status, count := range stats.CacheStatuses {
		p.metrics.SetCacheStatus(status, count)
	}

	for code, categories := range stats.HTTPResponses {
		for category, count := range categories {
			p.metrics.SetHTTPResponse(code, category, count)
		}
	}

	// Domain metrics - track "other" for untracked domains
	var otherRequests float64
	var otherBytesIn float64
	var otherBytesOut float64
	otherResponsesByCategory := make(map[string]int)

	for host, ports := range stats.DomainData {
		for port, data := range ports {
			domainKey := host + ":" + port

			// Determine if this domain should be tracked individually
			shouldTrack := false
			isUntracked := false

			if p.config.Global.TrackAllDomains {
				p.mu.Lock()
				if p.allDomainsSeen[domainKey] {
					// Already tracking this domain
					shouldTrack = true
				} else if len(p.allDomainsSeen) < p.config.Global.MaxDomains {
					// Room for new domain
					p.allDomainsSeen[domainKey] = true
					shouldTrack = true
				} else {
					// Max reached - will be aggregated to "other"
					shouldTrack = false
					isUntracked = true
				}
				p.mu.Unlock()

				if shouldTrack {
					// Update individual domain metrics
					p.metrics.UpdateAllDomains(
						host,
						port,
						float64(data.Requests),
						float64(data.BytesIn),
						float64(data.BytesOut),
						data.ResponsesByCategory,
					)
				} else if isUntracked {
					// Aggregate to "other"
					otherRequests += float64(data.Requests)
					otherBytesIn += float64(data.BytesIn)
					otherBytesOut += float64(data.BytesOut)
					for category, count := range data.ResponsesByCategory {
						otherResponsesByCategory[category] += count
					}
				}
			}

			// Check if domain is monitored
			monitoredDomain, isMonitored := p.config.IsMonitored(host, port)

			// If monitored, update extended metrics
			if isMonitored {
				// Calculate average duration
				avgDuration := 0.0
				if len(data.Durations) > 0 {
					sum := 0.0
					for _, d := range data.Durations {
						sum += d
					}
					avgDuration = sum / float64(len(data.Durations))
				}

				p50Duration := calculatePercentile(data.Durations, 0.50)
				p90Duration := calculatePercentile(data.Durations, 0.90)
				p95Duration := calculatePercentile(data.Durations, 0.95)
				p99Duration := calculatePercentile(data.Durations, 0.99)

				p.metrics.UpdateMonitoredDomain(
					host,
					port,
					monitoredDomain.Labels,
					float64(data.Requests),
					float64(data.BytesIn),
					float64(data.BytesOut),
					data.ResponsesByCode,
					avgDuration,
					p50Duration,
					p90Duration,
					p95Duration,
					p99Duration,
					data.CacheHits,
					data.CacheMisses,
				)
			}
		}
	}

	// Update "other" metric if we have untracked domains
	if p.config.Global.TrackAllDomains && otherRequests > 0 {
		p.metrics.UpdateAllDomains(
			"__other__",
			"0",
			otherRequests,
			otherBytesIn,
			otherBytesOut,
			otherResponsesByCategory,
		)

		p.mu.Lock()
		untrackedCount := 0
		for _, ports := range stats.DomainData {
			for range ports {
				untrackedCount++
			}
		}
		untrackedCount = untrackedCount - len(p.allDomainsSeen)
		p.mu.Unlock()

		if untrackedCount > 0 {
			log.Printf("Warning: %d domains not tracked individually (max_domains=%d reached). Aggregated to __other__",
				untrackedCount, p.config.Global.MaxDomains)
		}
	}
}

func categorizeHTTPCode(code string) string {
        if len(code) == 0 {
                return "unknown"
        }

        switch code[0] {
        case '2':
                return "2xx"
        case '3':
                return "3xx"
        case '4':
                return "4xx"
        case '5':
                return "5xx"
        default:
                return "other"
        }
}

func getDurationBucket(seconds float64) string {
        switch {
        case seconds < 0.1:
                return "0.1"
        case seconds < 0.5:
                return "0.5"
        case seconds < 1.0:
                return "1"
        case seconds < 5.0:
                return "5"
        case seconds < 10.0:
                return "10"
        default:
                return "10+"
        }
}

func calculatePercentile(durations []float64, percentile float64) float64 {
        if len(durations) == 0 {
                return 0
        }

        sorted := make([]float64, len(durations))
        copy(sorted, durations)
        sort.Float64s(sorted)

        index := int(float64(len(sorted)) * percentile)
        if index >= len(sorted) {
                index = len(sorted) - 1
        }

        return sorted[index]
}
