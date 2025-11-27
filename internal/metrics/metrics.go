package metrics

import (
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	// Global metrics
	connectionsTotal       prometheus.Counter
	requestDurationTotal   *prometheus.CounterVec
	cacheStatusTotal       *prometheus.CounterVec
	httpResponsesTotal     *prometheus.CounterVec

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

	// Global counters
	m.connectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "squid_connections_total",
		Help: "Total number of connections",
	})

	m.requestDurationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_request_duration_seconds_total",
			Help: "Total request duration in seconds by interval",
		},
		[]string{"interval"},
	)

	m.cacheStatusTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_cache_status_total",
			Help: "Total number of requests by cache status",
		},
		[]string{"status"},
	)

	m.httpResponsesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "squid_http_responses_total",
			Help: "Total number of HTTP responses by status code and category",
		},
		[]string{"code", "category"},
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
		// Global counters
		m.connectionsTotal,
		m.requestDurationTotal,
		m.cacheStatusTotal,
		m.httpResponsesTotal,
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

// Global metrics methods
func (m *Metrics) SetConnections(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := "connections"
	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.connectionsTotal.Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetRequestDuration(interval string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("duration", interval)
	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.requestDurationTotal.WithLabelValues(interval).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetCacheStatus(status string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("cache", status)
	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.cacheStatusTotal.WithLabelValues(status).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

func (m *Metrics) SetHTTPResponse(code, category string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := makeKey("http", code, category)
	lastValue := m.lastSeen[key]
	delta := float64(count) - lastValue
	if delta > 0 {
		m.httpResponsesTotal.WithLabelValues(code, category).Add(delta)
	}
	m.lastSeen[key] = float64(count)
}

// UpdateAllDomains updates metrics for all domains tracking
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

	// Durations (uses baseLabels - host, port, custom_labels)
	m.monitoredDomainsAvgDuration.WithLabelValues(baseLabels...).Set(avgDuration)
	m.monitoredDomainsP50Duration.WithLabelValues(baseLabels...).Set(p50Duration)
	m.monitoredDomainsP90Duration.WithLabelValues(baseLabels...).Set(p90Duration)
	m.monitoredDomainsP95Duration.WithLabelValues(baseLabels...).Set(p95Duration)
	m.monitoredDomainsP99Duration.WithLabelValues(baseLabels...).Set(p99Duration)

	// Cache hit ratio (uses baseLabels - host, port, custom_labels)
	totalCache := cacheHits + cacheMisses
	if totalCache > 0 {
		ratio := float64(cacheHits) / float64(totalCache)
		m.monitoredDomainsCacheHitRatio.WithLabelValues(baseLabels...).Set(ratio)
	}
}
