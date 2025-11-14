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

package metrics

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	// Existing gauges (backwards compatibility)
	connectionsTotal              prometheus.Gauge
	requestDurationTotal          *prometheus.GaugeVec
	cacheStatusTotal              *prometheus.GaugeVec
	httpResponsesTotal            *prometheus.GaugeVec
	httpResponsesByCategory       *prometheus.GaugeVec
	domainRequestsTotal           *prometheus.GaugeVec
	domainHTTPResponses           *prometheus.GaugeVec
	domainHTTPResponsesByCategory *prometheus.GaugeVec
	domainAvgDuration             *prometheus.GaugeVec

	// New counters
	connectionsCounter                   prometheus.Counter
	requestDurationCounter               *prometheus.CounterVec
	cacheStatusCounter                   *prometheus.CounterVec
	httpResponsesCounter                 *prometheus.CounterVec
	httpResponsesByCategoryCounter       *prometheus.CounterVec
	domainRequestsCounter                *prometheus.CounterVec
	domainHTTPResponsesCounter           *prometheus.CounterVec
	domainHTTPResponsesByCategoryCounter *prometheus.CounterVec

	// Basic metrics for ALL domains
	allDomainsRequestsCounter      *prometheus.CounterVec
	allDomainsHTTPResponsesCounter *prometheus.CounterVec
	allDomainsBytesCounter         *prometheus.CounterVec

	// Extended metrics for MONITORED domains (with dynamic labels)
	monitoredDomainsRequestsCounter      *prometheus.CounterVec
	monitoredDomainsHTTPResponsesCounter *prometheus.CounterVec
	monitoredDomainsBytesCounter         *prometheus.CounterVec
	monitoredDomainsAvgDuration          *prometheus.GaugeVec
	monitoredDomainsP50Duration          *prometheus.GaugeVec
	monitoredDomainsP90Duration          *prometheus.GaugeVec
	monitoredDomainsP95Duration          *prometheus.GaugeVec
	monitoredDomainsP99Duration          *prometheus.GaugeVec
	monitoredDomainsCacheHitRatio        *prometheus.GaugeVec

	// Custom label keys for monitored domains
	customLabelKeys []string

	// State tracking
	lastSeen map[string]float64
	mu       sync.RWMutex
}

// NewMetrics creates metrics with optional custom label keys
func NewMetrics(customLabelKeys []string) *Metrics {
	m := &Metrics{
		lastSeen:        make(map[string]float64),
		customLabelKeys: customLabelKeys,
	}

	// Existing gauges
	m.connectionsTotal = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "squid_connections_total",
		Help: "Total number of connections (gauge)",
	})

	m.requestDurationTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_request_duration_seconds_total",
			Help: "Total request duration in seconds by interval (gauge)",
		},
		[]string{"interval"},
	)

	m.cacheStatusTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_cache_status_total",
			Help: "Total number of requests by cache status (gauge)",
		},
		[]string{"status"},
	)

	m.httpResponsesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_http_responses_total",
			Help: "Total number of HTTP responses by status code (gauge)",
		},
		[]string{"code", "category"},
	)

	m.httpResponsesByCategory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_http_responses_by_category_total",
			Help: "Total number of HTTP responses by category (gauge)",
		},
		[]string{"category"},
	)

	m.domainRequestsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_domain_requests_total",
			Help: "Total number of requests per domain (gauge)",
		},
		[]string{"host", "port"},
	)

	m.domainHTTPResponses = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_domain_http_responses_total",
			Help: "Total number of HTTP responses per domain by status code (gauge)",
		},
		[]string{"host", "port", "code", "category"},
	)

	m.domainHTTPResponsesByCategory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_domain_http_responses_by_category_total",
			Help: "Total number of HTTP responses per domain by category (gauge)",
		},
		[]string{"host", "port", "category"},
	)

	m.domainAvgDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_domain_avg_duration_seconds",
			Help: "Average request duration per domain in seconds",
		},
		[]string{"host", "port"},
	)

	// New counters
	m.connectionsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "squid_connections_counter_total",
		Help: "Total number of connections (counter)",
	})

	m.requestDurationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_request_duration_seconds_counter_total",
			Help: "Total request duration in seconds by interval (counter)",
		},
		[]string{"interval"},
	)

	m.cacheStatusCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_cache_status_counter_total",
			Help: "Total number of requests by cache status (counter)",
		},
		[]string{"status"},
	)

	m.httpResponsesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_http_responses_counter_total",
			Help: "Total number of HTTP responses by status code (counter)",
		},
		[]string{"code", "category"},
	)

	m.httpResponsesByCategoryCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_http_responses_by_category_counter_total",
			Help: "Total number of HTTP responses by category (counter)",
		},
		[]string{"category"},
	)

	m.domainRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_domain_requests_counter_total",
			Help: "Total number of requests per domain (counter)",
		},
		[]string{"host", "port"},
	)

	m.domainHTTPResponsesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_domain_http_responses_counter_total",
			Help: "Total number of HTTP responses per domain by status code (counter)",
		},
		[]string{"host", "port", "code", "category"},
	)

	m.domainHTTPResponsesByCategoryCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_domain_http_responses_by_category_counter_total",
			Help: "Total number of HTTP responses per domain by category (counter)",
		},
		[]string{"host", "port", "category"},
	)

	// All domains metrics
	m.allDomainsRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_all_domains_requests_total",
			Help: "Total requests for all domains (basic tracking)",
		},
		[]string{"host", "port"},
	)

	m.allDomainsHTTPResponsesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_all_domains_http_responses_total",
			Help: "HTTP responses for all domains by category",
		},
		[]string{"host", "port", "category"},
	)

	m.allDomainsBytesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_all_domains_bytes_total",
			Help: "Total bytes transferred for all domains",
		},
		[]string{"host", "port", "direction"},
	)

	// Monitored domains metrics with dynamic custom labels
	// Base labels: host, port + custom labels from config
	monitoredLabels := append([]string{"host", "port"}, customLabelKeys...)

	m.monitoredDomainsRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_monitored_domains_requests_total",
			Help: "Total requests for monitored domains (extended tracking)",
		},
		monitoredLabels,
	)

	monitoredHTTPLabels := append([]string{"host", "port", "code", "category"}, customLabelKeys...)
	m.monitoredDomainsHTTPResponsesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_monitored_domains_http_responses_total",
			Help: "HTTP responses for monitored domains with detailed breakdown",
		},
		monitoredHTTPLabels,
	)

	monitoredBytesLabels := append([]string{"host", "port", "direction"}, customLabelKeys...)
	m.monitoredDomainsBytesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_monitored_domains_bytes_total",
			Help: "Bytes transferred for monitored domains",
		},
		monitoredBytesLabels,
	)

	m.monitoredDomainsAvgDuration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_monitored_domains_duration_seconds_avg",
			Help: "Average request duration for monitored domains",
		},
		monitoredLabels,
	)

	m.monitoredDomainsP50Duration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_monitored_domains_duration_seconds_p50",
			Help: "50th percentile (median) request duration for monitored domains",
		},
		monitoredLabels,
	)

	m.monitoredDomainsP90Duration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_monitored_domains_duration_seconds_p90",
			Help: "90th percentile request duration for monitored domains",
		},
		monitoredLabels,
	)

	m.monitoredDomainsP95Duration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_monitored_domains_duration_seconds_p95",
			Help: "95th percentile request duration for monitored domains",
		},
		monitoredLabels,
	)

	m.monitoredDomainsP99Duration = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_monitored_domains_duration_seconds_p99",
			Help: "99th percentile request duration for monitored domains",
		},
		monitoredLabels,
	)

	m.monitoredDomainsCacheHitRatio = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "squid_monitored_domains_cache_hit_ratio",
			Help: "Cache hit ratio for monitored domains",
		},
		monitoredLabels,
	)

	// Register all
	prometheus.MustRegister(
		// Gauges
		m.connectionsTotal,
		m.requestDurationTotal,
		m.cacheStatusTotal,
		m.httpResponsesTotal,
		m.httpResponsesByCategory,
		m.domainRequestsTotal,
		m.domainHTTPResponses,
		m.domainHTTPResponsesByCategory,
		m.domainAvgDuration,
		// Counters
		m.connectionsCounter,
		m.requestDurationCounter,
		m.cacheStatusCounter,
		m.httpResponsesCounter,
		m.httpResponsesByCategoryCounter,
		m.domainRequestsCounter,
		m.domainHTTPResponsesCounter,
		m.domainHTTPResponsesByCategoryCounter,
		// All domains
		m.allDomainsRequestsCounter,
		m.allDomainsHTTPResponsesCounter,
		m.allDomainsBytesCounter,
		// Monitored domains
		m.monitoredDomainsRequestsCounter,
		m.monitoredDomainsHTTPResponsesCounter,
		m.monitoredDomainsBytesCounter,
		m.monitoredDomainsAvgDuration,
		m.monitoredDomainsP50Duration,
		m.monitoredDomainsP90Duration,
		m.monitoredDomainsP95Duration,
		m.monitoredDomainsP99Duration,
		m.monitoredDomainsCacheHitRatio,
	)

	return m
}

func makeKey(labels ...string) string {
	return strings.Join(labels, "|")
}

// Existing methods (keep for backwards compatibility)
func (m *Metrics) SetConnections(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := "connections"
	m.connectionsTotal.Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.connectionsCounter.Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetRequestDuration(interval string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("duration", interval)
	m.requestDurationTotal.WithLabelValues(interval).Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.requestDurationCounter.WithLabelValues(interval).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetCacheStatus(status string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("cache", status)
	m.cacheStatusTotal.WithLabelValues(status).Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.cacheStatusCounter.WithLabelValues(status).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetHTTPResponse(code, category string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("http", code, category)
	m.httpResponsesTotal.WithLabelValues(code, category).Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.httpResponsesCounter.WithLabelValues(code, category).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetHTTPResponseByCategory(category string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("http_cat", category)
	m.httpResponsesByCategory.WithLabelValues(category).Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.httpResponsesByCategoryCounter.WithLabelValues(category).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetDomainRequests(host, port string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("domain_req", host, port)
	m.domainRequestsTotal.WithLabelValues(host, port).Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.domainRequestsCounter.WithLabelValues(host, port).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetDomainHTTPResponse(host, port, code, category string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("domain_http", host, port, code, category)
	m.domainHTTPResponses.WithLabelValues(host, port, code, category).Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.domainHTTPResponsesCounter.WithLabelValues(host, port, code, category).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetDomainHTTPResponseByCategory(host, port, category string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("domain_http_cat", host, port, category)
	m.domainHTTPResponsesByCategory.WithLabelValues(host, port, category).Set(float64(count))

	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.domainHTTPResponsesByCategoryCounter.WithLabelValues(host, port, category).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetDomainAvgDuration(host, port string, duration float64) {
	m.domainAvgDuration.WithLabelValues(host, port).Set(duration)
}

// New methods for all domains tracking
func (m *Metrics) UpdateAllDomains(host, port string, requests, bytesIn, bytesOut float64, responsesByCategory map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("all_req", host, port)
	lastValue := m.lastSeen[key]
	delta := requests - lastValue
	if delta > 0 {
		m.allDomainsRequestsCounter.WithLabelValues(host, port).Add(delta)
	}
	m.lastSeen[key] = requests

	keyIn := makeKey("all_bytes_in", host, port)
	lastValueIn := m.lastSeen[keyIn]
	deltaIn := bytesIn - lastValueIn
	if deltaIn > 0 {
		m.allDomainsBytesCounter.WithLabelValues(host, port, "in").Add(deltaIn)
	}
	m.lastSeen[keyIn] = bytesIn

	keyOut := makeKey("all_bytes_out", host, port)
	lastValueOut := m.lastSeen[keyOut]
	deltaOut := bytesOut - lastValueOut
	if deltaOut > 0 {
		m.allDomainsBytesCounter.WithLabelValues(host, port, "out").Add(deltaOut)
	}
	m.lastSeen[keyOut] = bytesOut

	for category, count := range responsesByCategory {
		keyCat := makeKey("all_http_cat", host, port, category)
		lastValueCat := m.lastSeen[keyCat]
		deltaCat := float64(count) - lastValueCat
		if deltaCat > 0 {
			m.allDomainsHTTPResponsesCounter.WithLabelValues(host, port, category).Add(deltaCat)
		}
		m.lastSeen[keyCat] = float64(count)
	}
}

// buildLabelValues builds label values array in correct order
func (m *Metrics) buildLabelValues(host, port string, customLabels map[string]string) []string {
	values := []string{host, port}
	for _, key := range m.customLabelKeys {
		values = append(values, customLabels[key])
	}
	return values
}

// UpdateMonitoredDomain updates metrics for monitored domains
func (m *Metrics) UpdateMonitoredDomain(
	host, port string,
	customLabels map[string]string,
	requests, bytesIn, bytesOut float64,
	responsesByCode map[string]map[string]int,
	avgDuration, p50Duration, p90Duration, p95Duration, p99Duration float64,
	cacheHits, cacheMisses int,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Build base label values (host, port, custom labels)
	baseLabels := m.buildLabelValues(host, port, customLabels)

	// Requests
	key := makeKey("mon_req", host, port)
	lastValue := m.lastSeen[key]
	delta := requests - lastValue
	if delta > 0 {
		m.monitoredDomainsRequestsCounter.WithLabelValues(baseLabels...).Add(delta)
	}
	m.lastSeen[key] = requests

	// Bytes
	keyIn := makeKey("mon_bytes_in", host, port)
	lastValueIn := m.lastSeen[keyIn]
	deltaIn := bytesIn - lastValueIn
	if deltaIn > 0 {
		bytesInLabels := []string{host, port, "in"}
		for _, key := range m.customLabelKeys {
			bytesInLabels = append(bytesInLabels, customLabels[key])
		}
		m.monitoredDomainsBytesCounter.WithLabelValues(bytesInLabels...).Add(deltaIn)
	}
	m.lastSeen[keyIn] = bytesIn

	keyOut := makeKey("mon_bytes_out", host, port)
	lastValueOut := m.lastSeen[keyOut]
	deltaOut := bytesOut - lastValueOut
	if deltaOut > 0 {
		bytesOutLabels := []string{host, port, "out"}
		for _, key := range m.customLabelKeys {
			bytesOutLabels = append(bytesOutLabels, customLabels[key])
		}
		m.monitoredDomainsBytesCounter.WithLabelValues(bytesOutLabels...).Add(deltaOut)
	}
	m.lastSeen[keyOut] = bytesOut

	// HTTP responses
	for code, categories := range responsesByCode {
		for category, count := range categories {
			keyCode := makeKey("mon_http", host, port, code, category)
			lastValueCode := m.lastSeen[keyCode]
			deltaCode := float64(count) - lastValueCode
			if deltaCode > 0 {
				httpLabels := []string{host, port, code, category}
				for _, key := range m.customLabelKeys {
					httpLabels = append(httpLabels, customLabels[key])
				}
				m.monitoredDomainsHTTPResponsesCounter.WithLabelValues(httpLabels...).Add(deltaCode)
			}
			m.lastSeen[keyCode] = float64(count)
		}
	}

	// Durations
	m.monitoredDomainsAvgDuration.WithLabelValues(baseLabels...).Set(avgDuration)
	m.monitoredDomainsP50Duration.WithLabelValues(baseLabels...).Set(p50Duration)
	m.monitoredDomainsP90Duration.WithLabelValues(baseLabels...).Set(p90Duration)
	m.monitoredDomainsP95Duration.WithLabelValues(baseLabels...).Set(p95Duration)
	m.monitoredDomainsP99Duration.WithLabelValues(baseLabels...).Set(p99Duration)

	// Cache hit ratio
	totalCache := cacheHits + cacheMisses
	if totalCache > 0 {
		ratio := float64(cacheHits) / float64(totalCache)
		m.monitoredDomainsCacheHitRatio.WithLabelValues(baseLabels...).Set(ratio)
	}
}
