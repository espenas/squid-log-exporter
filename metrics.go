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
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

	// Write domain-specific metrics if we have monitored domains
	if len(mc.domainStats) > 0 {
		// Request count per domain
		if _, err := fmt.Fprintf(tmpfile, "# HELP squid_domain_requests_total Total requests per monitored domain\n"); err != nil {
			return fmt.Errorf("failed to write domain help: %v", err)
		}
		if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_domain_requests_total counter\n"); err != nil {
			return fmt.Errorf("failed to write domain type: %v", err)
		}

		// Sort domain names for consistent output
		var hostPorts []string
		for hostPort := range mc.domainStats {
			hostPorts = append(hostPorts, hostPort)
		}
		sort.Strings(hostPorts)

		for _, hostPort := range hostPorts {
			stats := mc.domainStats[hostPort]
			parts := strings.Split(hostPort, ":")
			if len(parts) != 2 {
				continue
			}
			host := parts[0]
			port := parts[1]

			// Build label string with custom labels
			labelStr := fmt.Sprintf("host=\"%s\",port=\"%s\"", host, port)
			
			if len(stats.labels) > 0 {
				var labelKeys []string
				for key := range stats.labels {
					labelKeys = append(labelKeys, key)
				}
				sort.Strings(labelKeys)
				
				for _, key := range labelKeys {
					labelStr += fmt.Sprintf(",%s=\"%s\"", key, stats.labels[key])
				}
			}

			if _, err := fmt.Fprintf(tmpfile, "squid_domain_requests_total{%s} %d\n",
				labelStr, stats.count); err != nil {
				return fmt.Errorf("failed to write domain metrics: %v", err)
			}
		}
		if _, err := fmt.Fprintln(tmpfile); err != nil {
			return fmt.Errorf("failed to write separator: %v", err)
		}

		// Average duration per domain
		if _, err := fmt.Fprintf(tmpfile, "# HELP squid_domain_avg_duration_seconds Average connection duration per domain\n"); err != nil {
			return fmt.Errorf("failed to write domain avg help: %v", err)
		}
		if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_domain_avg_duration_seconds gauge\n"); err != nil {
			return fmt.Errorf("failed to write domain avg type: %v", err)
		}

		for _, hostPort := range hostPorts {
			stats := mc.domainStats[hostPort]
			parts := strings.Split(hostPort, ":")
			if len(parts) != 2 {
				continue
			}
			host := parts[0]
			port := parts[1]

			// Build label string with custom labels
			labelStr := fmt.Sprintf("host=\"%s\",port=\"%s\"", host, port)
			
			if len(stats.labels) > 0 {
				var labelKeys []string
				for key := range stats.labels {
					labelKeys = append(labelKeys, key)
				}
				sort.Strings(labelKeys)
				
				for _, key := range labelKeys {
					labelStr += fmt.Sprintf(",%s=\"%s\"", key, stats.labels[key])
				}
			}

			avgDuration := stats.totalDuration / float64(stats.count)
			if _, err := fmt.Fprintf(tmpfile, "squid_domain_avg_duration_seconds{%s} %.6f\n",
				labelStr, avgDuration); err != nil {
				return fmt.Errorf("failed to write domain avg metrics: %v", err)
			}
		}
		if _, err := fmt.Fprintln(tmpfile); err != nil {
			return fmt.Errorf("failed to write separator: %v", err)
		}

		// Max duration per domain
		if _, err := fmt.Fprintf(tmpfile, "# HELP squid_domain_max_duration_seconds Longest connection duration per domain\n"); err != nil {
			return fmt.Errorf("failed to write domain max help: %v", err)
		}
		if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_domain_max_duration_seconds gauge\n"); err != nil {
			return fmt.Errorf("failed to write domain max type: %v", err)
		}

		for _, hostPort := range hostPorts {
			stats := mc.domainStats[hostPort]
			parts := strings.Split(hostPort, ":")
			if len(parts) != 2 {
				continue
			}
			host := parts[0]
			port := parts[1]

			// Build label string with custom labels
			labelStr := fmt.Sprintf("host=\"%s\",port=\"%s\"", host, port)
			
			if len(stats.labels) > 0 {
				var labelKeys []string
				for key := range stats.labels {
					labelKeys = append(labelKeys, key)
				}
				sort.Strings(labelKeys)
				
				for _, key := range labelKeys {
					labelStr += fmt.Sprintf(",%s=\"%s\"", key, stats.labels[key])
				}
			}

			if _, err := fmt.Fprintf(tmpfile, "squid_domain_max_duration_seconds{%s} %.6f\n",
				labelStr, stats.maxDuration); err != nil {
				return fmt.Errorf("failed to write domain max metrics: %v", err)
			}
		}
		if _, err := fmt.Fprintln(tmpfile); err != nil {
			return fmt.Errorf("failed to write separator: %v", err)
		}

		// Min duration per domain
		if _, err := fmt.Fprintf(tmpfile, "# HELP squid_domain_min_duration_seconds Shortest connection duration per domain\n"); err != nil {
			return fmt.Errorf("failed to write domain min help: %v", err)
		}
		if _, err := fmt.Fprintf(tmpfile, "# TYPE squid_domain_min_duration_seconds gauge\n"); err != nil {
			return fmt.Errorf("failed to write domain min type: %v", err)
		}

		for _, hostPort := range hostPorts {
			stats := mc.domainStats[hostPort]
			parts := strings.Split(hostPort, ":")
			if len(parts) != 2 {
				continue
			}
			host := parts[0]
			port := parts[1]

			// Build label string with custom labels
			labelStr := fmt.Sprintf("host=\"%s\",port=\"%s\"", host, port)
			
			if len(stats.labels) > 0 {
				var labelKeys []string
				for key := range stats.labels {
					labelKeys = append(labelKeys, key)
				}
				sort.Strings(labelKeys)
				
				for _, key := range labelKeys {
					labelStr += fmt.Sprintf(",%s=\"%s\"", key, stats.labels[key])
				}
			}

			if _, err := fmt.Fprintf(tmpfile, "squid_domain_min_duration_seconds{%s} %.6f\n",
				labelStr, stats.minDuration); err != nil {
				return fmt.Errorf("failed to write domain min metrics: %v", err)
			}
		}
		if _, err := fmt.Fprintln(tmpfile); err != nil {
			return fmt.Errorf("failed to write separator: %v", err)
		}
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
