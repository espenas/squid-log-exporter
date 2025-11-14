# Squid Log Exporter

Prometheus exporter for Squid access logs with support for domain-level metrics, custom labels, and position tracking.

## Features

- ✅ **Counter metrics** - Proper monotonically increasing counters for rate/increase calculations
- ✅ **Position tracking** - Incremental log parsing (no re-parsing on restart)
- ✅ **Log rotation support** - Automatic detection via inode tracking
- ✅ **Configurable log formats** - Supports standard Squid and custom formats
- ✅ **All domains tracking** - Basic metrics for every domain (requests, bytes, HTTP categories)
- ✅ **Monitored domains** - Extended metrics with custom labels and latency tracking
- ✅ **Pattern matching** - Bulk configuration via wildcards
- ✅ **Performance metrics** - P50/P90/P95/P99 latency, cache hit ratio
- ✅ **Team/Service labels** - Cost allocation and team dashboards
- ✅ **"Other" aggregation** - No data loss when max_domains is reached

## Metrics

### Global Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `squid_connections_total` | Counter | Total number of connections |
| `squid_request_duration_seconds_total` | Counter | Total request duration by interval |
| `squid_cache_status_total` | Counter | Requests by cache status (HIT/MISS/etc) |
| `squid_http_responses_total` | Counter | HTTP responses by status code and category |

**Note:** All global metrics are Counters. Use `rate()` or `increase()` in PromQL queries.

### All Domains Metrics (Basic)

Basic tracking for all domains (up to `max_domains`). Provides overview without custom labels.

| Metric | Labels | Description |
|--------|--------|-------------|
| `squid_all_domains_requests_total` | `host`, `port` | Total requests |
| `squid_all_domains_http_responses_total` | `host`, `port`, `category` | HTTP responses by category (2xx, 4xx, 5xx) |
| `squid_all_domains_bytes_total` | `host`, `port`, `direction` | Bytes transferred (in/out) |

**Note:**
- When `max_domains` limit is reached, additional domains are aggregated into a special `{host="__other__",port="0"}` metric
- Use this to monitor if you need to increase `max_domains`
- Monitored domains are always tracked individually, regardless of `max_domains`

**Example with max_domains reached:**
```promql
# Individual domains (up to max_domains)
squid_all_domains_requests_total{host="api.example.com",port="443"} 1000
squid_all_domains_requests_total{host="web.example.com",port="443"} 2000
...

# All remaining domains aggregated
squid_all_domains_requests_total{host="__other__",port="0"} 5000
```

**All domains tracking does NOT include:**
- Individual HTTP status codes (only categories: 2xx, 4xx, 5xx)
- Latency metrics
- Custom labels

Use `monitored_domains` for detailed tracking of important domains.

### Monitored Domains Metrics (Extended)

Full tracking with custom labels for your most important domains. Includes detailed HTTP codes and latency percentiles.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `squid_monitored_domains_requests_total` | Counter | `host`, `port`, *custom labels* | Total requests with business context |
| `squid_monitored_domains_http_responses_total` | Counter | `host`, `port`, `code`, `category`, *custom labels* | Detailed HTTP responses with exact codes |
| `squid_monitored_domains_bytes_total` | Counter | `host`, `port`, `direction`, *custom labels* | Bytes transferred (in/out) |
| `squid_monitored_domains_duration_seconds_avg` | Gauge | `host`, `port`, *custom labels* | Average latency |
| `squid_monitored_domains_duration_seconds_p50` | Gauge | `host`, `port`, *custom labels* | Median latency |
| `squid_monitored_domains_duration_seconds_p90` | Gauge | `host`, `port`, *custom labels* | 90th percentile latency |
| `squid_monitored_domains_duration_seconds_p95` | Gauge | `host`, `port`, *custom labels* | 95th percentile latency |
| `squid_monitored_domains_duration_seconds_p99` | Gauge | `host`, `port`, *custom labels* | 99th percentile latency |
| `squid_monitored_domains_cache_hit_ratio` | Gauge | `host`, `port`, *custom labels* | Cache effectiveness (0-1) |

**Custom labels** are defined per domain in your configuration (e.g., `team`, `service`, `environment`, `critical`).

## Installation

### Build from source
```bash
git clone https://github.com/espenas/squid-log-exporter.git
cd squid-log-exporter
go build -o squid-log-exporter ./cmd/squid-log-exporter
```

### Install
```bash
# Create user
sudo useradd --system --no-create-home --shell /bin/false squid-exporter

# Install binary
sudo cp squid-log-exporter /usr/local/bin/
sudo chmod +x /usr/local/bin/squid-log-exporter

# Create directories
sudo mkdir -p /etc/squid-log-exporter
sudo mkdir -p /var/lib/squid-log-exporter
sudo chown squid-exporter:squid-exporter /var/lib/squid-log-exporter
sudo chmod 755 /var/lib/squid-log-exporter

# Install config
sudo cp examples/config.yaml /etc/squid-log-exporter/config.yaml

# Install systemd service
sudo cp deployments/systemd/squid-log-exporter.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable squid-log-exporter
sudo systemctl start squid-log-exporter
```

## Configuration

### Basic Configuration

Create `/etc/squid-log-exporter/config.yaml`:
```yaml
global:
  track_all_domains: true    # Basic tracking for all domains
  max_domains: 10000         # Limit to prevent memory issues

# Log format is optional - defaults to squid_native
# Only specify if you use a custom format

monitored_domains:
  - host: "api.example.com"
    port: "443"
    labels:
      team: "backend"
      service: "api"
      environment: "prod"
      critical: "true"
  
  - host: "web.example.com"
    port: "443"
    labels:
      team: "frontend"
      service: "web"
      environment: "prod"

domain_patterns:
  - pattern: "*.prod.example.com"
    labels:
      environment: "prod"
  
  - pattern: "*.dev.example.com"
    labels:
      environment: "dev"
```

### Custom Labels

You can define any custom labels you want. Common examples:

- `team` - Team ownership (backend, frontend, data, etc.)
- `service` - Service name (api, web, cache, etc.)
- `environment` - Environment (prod, staging, dev)
- `critical` - Business criticality (true/false)
- `cost_center` - For cost allocation
- `region` - Geographical region

Custom labels appear in all `squid_monitored_domains_*` metrics.

### Log Format Configuration

#### Default (Standard Squid Native Format)

No configuration needed! The exporter uses standard Squid access.log format by default.

**Standard Squid format:**
```
1234567890.123 100 127.0.0.1 TCP_MISS/200 1234 GET http://example.com/ - HIER_DIRECT/93.184.216.34 text/html
```

#### Custom Log Format

If your Squid uses a custom log format, you can configure it:
```yaml
global:
  track_all_domains: true

log_format:
  type: "custom"
  fields:
    timestamp: 0       # 2025-10-29T12:21:11.832
    duration: 1        # 91 (milliseconds)
    client_ip: 2       # 123.10.142.141
    result_code: 3     # TCP_TUNNEL/200
    bytes: 4           # 1279
    method: 5          # CONNECT
    url: 6             # foo.example.com:443
    hierarchy: 7       # HIER_DIRECT/14.126.114.143
    content_type: 8    # -
    user_agent: 9      # "Java/1.8.0_462"
    referer: 10        # "-"
  timestamp_format: "2006-01-02T15:04:05.000"
  duration_unit: "ms"

monitored_domains:
  - host: "foo.example.com"
    port: "443"
    labels:
      service: "api"
```

**Example custom format log line:**
```
2025-10-29T12:21:11.832 91 123.10.142.141 TCP_TUNNEL/200 1279 CONNECT foo.example.com:443 HIER_DIRECT/14.126.114.143 - "Java/1.8.0_462" "-"
```

#### Supported Log Format Types

| Type | Description | Configuration Required |
|------|-------------|----------------------|
| `squid_native` | Standard Squid access.log format (default) | ❌ No |
| `squid_combined` | Squid with referer and user_agent | ❌ No |
| `custom` | Your custom log format | ✅ Yes - define fields |

#### Custom Format Fields

Required fields for custom format:
- `timestamp` - Log entry timestamp
- `duration` - Request duration
- `result_code` - Cache status and HTTP code (e.g., TCP_MISS/200)
- `bytes` - Bytes transferred
- `method` - HTTP method (GET, POST, CONNECT, etc.)
- `url` - Requested URL or host:port

Optional fields:
- `client_ip` - Client IP address
- `hierarchy` - Squid hierarchy (HIER_DIRECT, etc.)
- `content_type` - Content type
- `user_agent` - User agent string
- `referer` - HTTP referer

#### Timestamp Formats

Use Go time layout format:

| Example | Format String |
|---------|---------------|
| `2025-10-29T12:21:11.832` | `2006-01-02T15:04:05.000` |
| `2025-10-29 12:21:11` | `2006-01-02 15:04:05` |
| `29/Oct/2025:12:21:11` | `02/Jan/2006:15:04:05` |
| Unix timestamp | `unix` |

#### Duration Units

- `ms` - Milliseconds (default for Squid)
- `s` - Seconds

## Usage
```bash
squid-log-exporter \
  --listen-address=:9448 \
  --log-file=/var/log/squid/access.log \
  --config=/etc/squid-log-exporter/config.yaml \
  --position-file=/var/lib/squid-log-exporter/position.json \
  --interval=60s
```

### Command-line Options

| Flag | Default | Description |
|------|---------|-------------|
| `--listen-address` | `:9448` | HTTP server listen address |
| `--metrics-path` | `/metrics` | Path to expose metrics |
| `--log-file` | `/var/log/squid/access.log` | Squid access log file path |
| `--config` | `/etc/squid-log-exporter/config.yaml` | Configuration file path |
| `--position-file` | `/var/lib/squid-log-exporter/position.json` | Position tracking file |
| `--interval` | `60s` | Log parsing interval |
| `--version` | - | Show version information |

## Prometheus Configuration
```yaml
scrape_configs:
  - job_name: 'squid-log-exporter'
    static_configs:
      - targets: ['localhost:9448']
    scrape_interval: 60s  # Match your --interval setting
```

## Example Queries

### Basic Queries (All Domains)
```promql
# Top 10 domains by request rate (excluding "other")
topk(10, rate(squid_all_domains_requests_total{host!="__other__"}[5m]))

# Check if max_domains is too low (percentage in "other")
(
  squid_all_domains_requests_total{host="__other__"}
  /
  sum(squid_all_domains_requests_total)
) * 100

# Total tracked domains
count(squid_all_domains_requests_total{host!="__other__"})

# Untracked traffic volume
rate(squid_all_domains_requests_total{host="__other__"}[5m])

# Total bandwidth by domain
topk(10,
  sum by(host) (
    rate(squid_all_domains_bytes_total[1h])
  )
)

# Error rate per domain
sum by(host) (
  rate(squid_all_domains_http_responses_total{category=~"4xx|5xx"}[5m])
)

# HTTP responses by category (aggregated from detailed metric)
sum by(category) (rate(squid_http_responses_total[5m]))
```

### Advanced Queries (Monitored Domains)
```promql
# Request rate per team
sum by(team) (
  rate(squid_monitored_domains_requests_total[5m])
)

# Error rate for critical services
(
  sum(rate(squid_monitored_domains_http_responses_total{critical="true",category=~"4xx|5xx"}[5m]))
  /
  sum(rate(squid_monitored_domains_http_responses_total{critical="true"}[5m]))
) * 100

# P95 latency for production services
squid_monitored_domains_duration_seconds_p95{environment="prod"}

# P50 (median) latency by service
squid_monitored_domains_duration_seconds_p50{environment="prod"}

# Cache hit ratio for critical services
squid_monitored_domains_cache_hit_ratio{critical="true"}

# Monthly bandwidth per team (GB)
sum by(team) (
  rate(squid_monitored_domains_bytes_total[1h])
) * 3600 * 24 * 30 / 1024 / 1024 / 1024

# 404 errors per service
sum by(service) (
  rate(squid_monitored_domains_http_responses_total{code="404"}[5m])
)

# Services with high latency
squid_monitored_domains_duration_seconds_p99 > 2
```

### Alerting Examples
```promql
# Alert: High error rate
(
  sum(rate(squid_monitored_domains_http_responses_total{critical="true",category="5xx"}[5m]))
  /
  sum(rate(squid_monitored_domains_http_responses_total{critical="true"}[5m]))
) > 0.05  # 5% error rate

# Alert: High latency
squid_monitored_domains_duration_seconds_p95{critical="true"} > 1  # Over 1 second

# Alert: Low cache hit ratio
squid_monitored_domains_cache_hit_ratio{critical="true"} < 0.5  # Below 50%

# Alert: Too many untracked domains
(
  squid_all_domains_requests_total{host="__other__"}
  /
  sum(squid_all_domains_requests_total)
) > 0.2  # Over 20% of traffic is untracked
```

## Architecture

### Metric Levels

The exporter provides three levels of metrics:

1. **Global metrics** - Aggregate statistics across all traffic
2. **All domains** - Basic per-domain tracking (limited by `max_domains`)
3. **Monitored domains** - Extended tracking with custom labels

### Why Two Domain Levels?

- **All domains**: Discover which domains are most active
- **Monitored domains**: Deep dive into important domains

**Workflow:**
1. Use `squid_all_domains_requests_total` to find top domains
2. Add important domains to `monitored_domains` config
3. Get full metrics including latency and custom labels

### Performance Considerations

- **max_domains**: Limits cardinality for all_domains tracking
  - Default: 10000 domains
  - Adjust based on your Prometheus capacity
  
- **monitored_domains**: Keep under 100 for optimal performance
  - Each monitored domain creates multiple time series
  - Use patterns to reduce configuration

## Migration from 1.x

Version 2.0 removes old redundant metrics and introduces the all_domains/monitored_domains split.

### Breaking Changes

**Removed metrics:**
- All `*_total` gauge versions (replaced with counter-only metrics)
- All `*_counter_total` metrics (renamed to `*_total` counters)
- `squid_http_responses_by_category_total` (aggregate from `squid_http_responses_total` instead)
- `squid_domain_*` metrics (replaced by `squid_all_domains_*` and `squid_monitored_domains_*`)

**Config changes:**
- Removed `squid_group` label
- Added flexible custom `labels` per domain

### Migration Steps

1. **Update configuration**:
```yaml
# Old (1.x)
monitored_domains:
  - host: "api.example.com"
    squid_group: "production"  # REMOVED

# New (2.x)
monitored_domains:
  - host: "api.example.com"
    labels:
      environment: "prod"  # Custom labels
      service: "api"
      team: "backend"
```

2. **Update Grafana queries**:
```promql
# Old (gauge)
squid_connections_total

# New (counter - use rate)
rate(squid_connections_total[5m])

# Old (counter with _counter_ suffix)
rate(squid_domain_requests_counter_total{host="api.example.com"}[5m])

# New (cleaner counter name)
rate(squid_all_domains_requests_total{host="api.example.com"}[5m])

# Old (by category metric)
rate(squid_http_responses_by_category_total{category="2xx"}[5m])

# New (aggregate from detailed metric)
sum by(category) (rate(squid_http_responses_total[5m]))
```

3. **Update alerts** to use new metric names and add `rate()` where needed

## Troubleshooting

### Check position file
```bash
cat /var/lib/squid-log-exporter/position.json
```

### Reset position (re-parse from beginning)
```bash
sudo systemctl stop squid-log-exporter
sudo rm /var/lib/squid-log-exporter/position.json
sudo systemctl start squid-log-exporter
```

### Check metrics
```bash
# All metrics
curl localhost:9448/metrics | grep squid_

# Global metrics
curl localhost:9448/metrics | grep "squid_connections\|squid_http_responses\|squid_cache"

# All domains
curl localhost:9448/metrics | grep squid_all_domains

# Monitored domains
curl localhost:9448/metrics | grep squid_monitored_domains
```

### View logs
```bash
journalctl -u squid-log-exporter -f
```

### Verify log format
If you see parsing errors, verify your log format configuration:
```bash
# Check what format your Squid uses
sudo head -1 /var/log/squid/access.log

# Test with a single line
echo "YOUR_LOG_LINE_HERE" > test.log
./squid-log-exporter --log-file=test.log --config=config.yaml --interval=5s
```

### Common Issues

**Problem**: `failed to save position: permission denied`
```bash
Solution: Ensure position file directory is writable by the exporter user

sudo chown squid-exporter:squid-exporter /var/lib/squid-log-exporter
sudo chmod 755 /var/lib/squid-log-exporter

# Test permissions
sudo -u squid-exporter touch /var/lib/squid-log-exporter/test.tmp
```

**Problem**: Too many time series
```
Solution: Reduce max_domains or limit monitored_domains
```

**Problem**: Missing custom labels in metrics
```
Solution: Ensure labels are defined consistently across all monitored_domains
```

**Problem**: Latency metrics always zero
```
Solution: Check that domains are in monitored_domains (not just all_domains)
```

**Problem**: Large amount of traffic in `__other__` metric
```
Solution: Increase max_domains in config:
  global:
    max_domains: 50000  # Increase from default 10000
```

**Problem**: How to see which domains are being aggregated to __other__?
```
Solution: Temporarily increase max_domains or check Squid access logs directly.
Use this query to see tracked domain count:
  count(squid_all_domains_requests_total{host!="__other__"})
```

**Problem**: Important domain ending up in __other__
```
Solution: Add it to monitored_domains - monitored domains are ALWAYS tracked
individually regardless of max_domains limit:
  monitored_domains:
    - host: "important.example.com"
      port: "443"
      labels:
        critical: "true"
```

**Problem**: Metrics show scientific notation like `4.239318e+06`
```
This is normal for large numbers. It means 4,239,318 (4.2 million).
Grafana will display these in human-readable format automatically.
```

## License

GNU General Public License v3.0

## Contributing

Pull requests are welcome! Please ensure:
- Code follows existing style
- Tests pass
- Documentation is updated
