# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.0.0] - 2025-01-XX

### Added
- Flexible custom labels for monitored domains (replace fixed `squid_group`)
- P50 and P90 latency percentiles for monitored domains
- `__other__` aggregation for domains beyond `max_domains` limit
  - No data loss when max_domains is reached
  - Warning logs when domains are aggregated to __other__
- Split tracking: `all_domains` (basic) vs `monitored_domains` (extended)
- Better documentation with comprehensive query examples
- Alerting examples in README
- Migration guide from v1.x

### Changed
- **BREAKING**: Default listen port changed from `:55123` to `:9448`
- **BREAKING**: Monitored domains now use flexible `labels` map instead of fixed `squid_group`
- **BREAKING**: Removed all gauge/counter duplicate metrics
  - Old: `squid_connections_total` (gauge) + `squid_connections_counter_total` (counter)
  - New: `squid_connections_total` (counter only)
- **BREAKING**: All `*_counter_total` metrics renamed to `*_total` (counters)
- All domains metrics only track HTTP categories, not individual codes
- Improved atomic file writes for position tracking
- Position temp files now written to same directory as destination

### Removed
- **BREAKING**: All gauge versions of accumulating metrics
  - `squid_connections_total` (gauge)
  - `squid_request_duration_seconds_total` (gauge)
  - `squid_cache_status_total` (gauge)
  - `squid_http_responses_total` (gauge)
- **BREAKING**: `squid_http_responses_by_category_total` metric
  - Aggregate from `squid_http_responses_total` with PromQL instead
- **BREAKING**: `squid_group` label from all metrics and configuration
- **BREAKING**: Old `squid_domain_*` metrics (v1.x compatibility)
  - `squid_domain_requests_total`
  - `squid_domain_http_responses_total`
  - `squid_domain_avg_duration_seconds`

### Fixed
- Cross-device link error when writing position file
- Position file permissions handling

### Migration Notes

**Configuration changes required:**
```yaml
# Old (v1.x)
monitored_domains:
  - host: "api.example.com"
    squid_group: "production"

# New (v2.0)
monitored_domains:
  - host: "api.example.com"
    labels:
      environment: "prod"
      service: "api"
      team: "backend"
```

**Query changes required:**
```promql
# Old: Gauge metrics
squid_connections_total

# New: Counter metrics (use rate/increase)
rate(squid_connections_total[5m])

# Old: Category metric
squid_http_responses_by_category_total{category="2xx"}

# New: Aggregate from detailed metric
sum by(category) (rate(squid_http_responses_total[5m]))
```

See README.md "Migration from 1.x" section for complete migration guide.

## [1.0.0] - 2024-XX-XX

### Added
- Initial release
- Position tracking for incremental log parsing
- Support for standard Squid log formats
- Domain-level metrics
- Gauge and Counter metrics for backwards compatibility
- Log rotation support via inode tracking
- Configurable parsing intervals
- Prometheus metrics endpoint
- Systemd service support

[2.0.0]: https://github.com/espenas/squid-log-exporter/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/espenas/squid-log-exporter/releases/tag/v1.0.0
