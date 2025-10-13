# Squid Log Exporter

A lightweight and efficient metrics exporter for Squid proxy access logs. This tool parses Squid access logs and generates Prometheus-compatible metrics, helping you monitor your Squid proxy server's performance and usage patterns.

## Features

- Parses Squid access logs and generates Prometheus-compatible metrics
- Tracks HTTP response codes and their categories (2xx, 3xx, 4xx, 5xx). HTTP response codes are stored in a file to be able to count 0 instances.
- Monitors cache status (TCP_HIT, TCP_MISS, TCP_DENIED, TCP_TUNNEL). Cache statuses are stored in a file to be able to count 0 instances.
- Measures request durations in both milliseconds and seconds (TCP_TUNNEL is excluded from duration buckets)
- **Domain-specific monitoring** - Track requests, average/min/max duration for specific domains
- **Custom labels** - Add your own labels (orgid, instid, environment, etc.) to domain metrics for flexible grouping and filtering
- Maintains state across restarts using position tracking
- Supports file rotation
- Configurable through both JSON config files and command-line flags
- Error logging with retry mechanism for metric writes
- Atomic file operations for reliability

## Metrics

The exporter generates the following metrics:

### General Metrics
- `squid_connections_total`: Total number of connections processed
- `squid_request_duration_milliseconds_total`: Request counts by duration intervals (ms)
- `squid_request_duration_seconds_total`: Request counts by duration intervals (s)
- `squid_http_responses_total`: HTTP response counts by status code and category
- `squid_http_responses_by_category_total`: HTTP response counts aggregated by category
- `squid_cache_status_total`: Request counts by cache status

### Domain-Specific Metrics
- `squid_domain_requests_total{host,port,...}`: Total requests per monitored domain
- `squid_domain_avg_duration_seconds{host,port,...}`: Average connection duration per domain
- `squid_domain_max_duration_seconds{host,port,...}`: Longest connection duration per domain
- `squid_domain_min_duration_seconds{host,port,...}`: Shortest connection duration per domain

**Note:** Domain metrics include TCP_TUNNEL connections, as these represent the majority of modern HTTPS traffic. All domain metrics automatically include any custom labels you define.

## Installation
```bash
go get github.com/espenas/squid-log-exporter
```

Or clone the repository and build manually:

```bash
git clone https://github.com/espenas/squid-log-exporter.git
cd squid-log-exporter
go mod init squid-log-exporter
go mod tidy
go build
```

## Configuration

The exporter can be configured through both a JSON configuration file and command-line flags. Command-line flags take precedence over configuration file values.

### Main Configuration File

Example `config.json`:

```json
{
  "access_log_path": "/var/log/squid/access.log",
  "position_file_path": "/var/lib/squid_exporter/position.txt",
  "output_path": "/var/lib/squid_exporter/metrics.txt",
  "buffer_size": 65536,
  "log_errors": true,
  "retry_attempts": 3,
  "retry_delay": "1s",
  "log_file_path": "/var/lib/squid_exporter/squid_metrics.log",
  "known_codes_file_path": "/var/lib/squid_exporter/known_http_codes.txt",
  "known_status_file_path": "/var/lib/squid_exporter/known_status.txt",
  "monitored_domains_path": "/etc/squid_exporter/domains.yaml"
}
```

### Monitored Domains Configuration

Create a `domains.yaml` file to specify which domains to monitor. You can add custom labels to each domain for flexible grouping and filtering in your monitoring system.

Basic Example:

```yaml
monitored_targets:
  - host: "example.com:443"
  - host: "api.example.com:443"
  - host: "legacy.example.com:80"
```

Example with Custom Labels:

```yaml
monitored_targets:
  - host: "example.com:443"
    labels:
      orgid: "org123"
      environment: "production"
  
  - host: "api.example.com:443"
    labels:
      orgid: "org456"
      environment: "staging"
      region: "eu-west"
  
  - host: "test.example.com:443"
    labels:
      instid: "inst789"
      testid: "test001"
  
  - host: "simple.example.com:443"
    # No labels - completely optional
```

Custom Labels:

- Labels are completely optional - domains without labels work perfectly fine
- Use any label names you want (orgid, instid, environment, region, team, etc.)
- Each domain can have different labels
- Labels appear as Prometheus labels in the metrics output
- Great for grouping, filtering, and aggregating metrics in your monitoring dashboards

Important Domain Configuration Notes:

- Use exact domain:port combinations as they appear in Squid logs
- HTTPS connections typically use port 443
- HTTP connections typically use port 80
- No wildcard matching - each domain must be specified exactly

To find your most active domains:
```bash
awk '{print $7}' /var/log/squid/access.log | sort | uniq -c | sort -rn | head -20
```


### Command-line Flags

```
  -access-log string
        Path to Squid access log file
  -buffer-size int
        Buffer size for reading log file (in bytes)
  -config string
        Path to configuration file (JSON)
  -domains-config string
        Path to monitored domains YAML configuration file
  -log-errors
        Enable error logging (default true)
  -output string
        Path where metrics will be written
  -position-file string
        Path to file storing the last read position
  -retry-attempts int
        Number of retry attempts for writing metrics
  -retry-delay string
        Delay between retry attempts (e.g., '1s', '500ms')
  -version
        Print version information
```

## Usage

Basic usage with default configuration:

```bash
./squid-log-exporter
```

Using a configuration file:

```bash
./squid-log-exporter -config /path/to/config.json
```

With domain monitoring:

```bash
./squid-log-exporter -config /path/to/config.json -domains-config /path/to/domains.yaml
```

Override specific settings:

```bash
./squid-log-exporter -access-log /custom/path/access.log -buffer-size 131072
```

Run on a schedule (e.g., every 5 minutes with cron):

```bash
*/5 * * * * /usr/local/bin/squid-log-exporter -config /etc/squid_exporter/config.json
```

## Output Format

The exporter generates Prometheus-compatible metrics in a text file. Example output:

```
# HELP squid_connections_total Total number of connections
# TYPE squid_connections_total counter
squid_connections_total 3302

# HELP squid_request_duration_milliseconds_total Number of requests by duration interval in milliseconds
# TYPE squid_request_duration_milliseconds_total counter
squid_request_duration_milliseconds_total{interval="0-200"} 130
squid_request_duration_milliseconds_total{interval="200-400"} 4
squid_request_duration_milliseconds_total{interval="400-600"} 1
squid_request_duration_milliseconds_total{interval="600-800"} 0
squid_request_duration_milliseconds_total{interval="800-1000"} 0
squid_request_duration_milliseconds_total{interval="over1000"} 1

# HELP squid_request_duration_seconds_total Number of requests by duration interval in seconds
# TYPE squid_request_duration_seconds_total counter
squid_request_duration_seconds_total{interval="0-1"} 135
squid_request_duration_seconds_total{interval="1-2"} 1
squid_request_duration_seconds_total{interval="2-3"} 0
squid_request_duration_seconds_total{interval="3-4"} 0
squid_request_duration_seconds_total{interval="4-5"} 0
squid_request_duration_seconds_total{interval="over5"} 0

# HELP squid_domain_requests_total Total requests per monitored domain
# TYPE squid_domain_requests_total counter
squid_domain_requests_total{host="example.com",port="443",environment="production",orgid="org123"} 1523
squid_domain_requests_total{host="api.example.com",port="443",environment="staging",orgid="org456",region="eu-west"} 456
squid_domain_requests_total{host="test.example.com",port="443",instid="inst789",testid="test001"} 234
squid_domain_requests_total{host="simple.example.com",port="443"} 100

# HELP squid_domain_avg_duration_seconds Average connection duration per domain
# TYPE squid_domain_avg_duration_seconds gauge
squid_domain_avg_duration_seconds{host="example.com",port="443",environment="production",orgid="org123"} 2.456789
squid_domain_avg_duration_seconds{host="api.example.com",port="443",environment="staging",orgid="org456",region="eu-west"} 1.234567

# HELP squid_domain_max_duration_seconds Longest connection duration per domain
# TYPE squid_domain_max_duration_seconds gauge
squid_domain_max_duration_seconds{host="example.com",port="443",environment="production",orgid="org123"} 45.123456

# HELP squid_domain_min_duration_seconds Shortest connection duration per domain
# TYPE squid_domain_min_duration_seconds gauge
squid_domain_min_duration_seconds{host="example.com",port="443",environment="production",orgid="org123"} 0.089123

# HELP squid_http_responses_total Total number of HTTP responses by status code and category
# TYPE squid_http_responses_total counter
squid_http_responses_total{code="000",category="0xx"} 0
squid_http_responses_total{code="200",category="2xx"} 3289
squid_http_responses_total{code="403",category="4xx"} 4
squid_http_responses_total{code="404",category="4xx"} 9
squid_http_responses_total{code="422",category="4xx"} 0
squid_http_responses_total{code="503",category="5xx"} 0

# HELP squid_http_responses_by_category_total Total number of HTTP responses by status code category
# TYPE squid_http_responses_by_category_total counter
squid_http_responses_by_category_total{category="0xx"} 0
squid_http_responses_by_category_total{category="2xx"} 3289
squid_http_responses_by_category_total{category="4xx"} 13
squid_http_responses_by_category_total{category="5xx"} 0

# HELP squid_cache_status_total Total number of requests by cache status
# TYPE squid_cache_status_total counter
squid_cache_status_total{status="TCP_DENIED"} 4
squid_cache_status_total{status="TCP_MISS"} 132
squid_cache_status_total{status="TCP_TUNNEL"} 3166
```

## Use Cases for Custom Labels

Custom labels enable powerful filtering and aggregation in Prometheus queries:

```proql
# Total requests per organization
sum by (orgid) (squid_domain_requests_total)

# Average duration for production environments
avg(squid_domain_avg_duration_seconds{environment="production"})

# Requests per region
sum by (region) (squid_domain_requests_total)

# Compare test instances
squid_domain_requests_total{testid=~"test.*"}
```

### Project Structure
```
squid-log-exporter/
├── main.go          # Entry point
├── types.go         # Type definitions
├── config.go        # Configuration handling
├── collector.go     # Metrics collector
├── parser.go        # Log parsing logic
├── metrics.go       # Metrics output
├── go.mod
├── go.sum
└── README.md
```

## Requirements

- Go 1.16 or higher
- Read access to Squid access log file
- Write access to output directory
- gopkg.in/yaml.v2 (for domain configuration)

## ToDo

* Make it a service
* Better duration statistics
* Support for custom log formats

## License

This project is licensed under the GNU General Public License v3.0 - see the included license notice for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Author

Espen Stefansen <espenas+github@gmail.com>
