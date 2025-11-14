# Changelog

## [2.0.0] - 2025-01-XX

### Added
- **Counter metrics** in addition to gauges for proper rate/increase calculations
- **Position tracking** to avoid re-parsing entire log files on restart
- **Configuration file support** for monitoring specific domains with custom labels
- **All domains tracking** with basic metrics (configurable)
- **Extended metrics** for monitored domains:
  - Custom labels (team, service, environment, critical)
  - P95 and P99 latency percentiles
  - Cache hit ratio
  - Per-direction bytes (in/out)
- **Pattern-based domain matching** for bulk configuration
- **Log rotation detection** via inode tracking
- **Graceful shutdown** handling

### Changed
- Metrics now use delta calculation for counters
- Parser incremental (only reads new lines since last parse)
- Improved error handling and logging

### Backwards Compatibility
- All existing gauge metrics retained
- Existing Grafana dashboards will continue to work
- New counter metrics have `_counter_` suffix to avoid conflicts

## [1.0.0] - Previous

### Initial release
- Basic Squid log parsing
- Gauge-based metrics
- Domain-level statistics
