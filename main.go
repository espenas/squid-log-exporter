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
    "log"
)

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

    if err := collector.writeMetricsWithRetry(codeCounts, cacheCounts, totalConnections, durationCounts); err != nil {
        log.Fatalf("Failed to write metrics: %v", err)
    }

    // Reset domain stats after writing metrics
    collector.domainStats = make(map[string]*DomainStats)

    fmt.Printf("Successfully processed %d connections with %d HTTP status codes and %d cache statuses\n",
        totalConnections, len(codeCounts), len(cacheCounts))

    if len(collector.domainStats) > 0 {
        fmt.Printf("Monitored %d domains\n", len(collector.domainStats))
    }
}
