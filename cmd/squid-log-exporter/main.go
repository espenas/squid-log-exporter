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
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"squid-log-exporter/internal/config"
	"squid-log-exporter/internal/metrics"
	"squid-log-exporter/internal/parser"
)

var (
	version = "2.0.0"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		listenAddr   = flag.String("listen-address", ":9448", "The address to listen on for HTTP requests")
		metricsPath  = flag.String("metrics-path", "/metrics", "Path under which to expose metrics")
		logFile      = flag.String("log-file", "/var/log/squid/access.log", "Path to Squid access log")
		configFile   = flag.String("config", "/etc/squid-log-exporter/config.yaml", "Path to configuration file")
		positionFile = flag.String("position-file", "/var/lib/squid-log-exporter/position.json", "Path to position tracking file")
		interval     = flag.Duration("interval", 60*time.Second, "Interval for parsing logs")
		showVersion  = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *showVersion {
		log.Printf("squid-log-exporter version %s (commit: %s, built: %s)", version, commit, date)
		os.Exit(0)
	}

	log.Printf("Starting squid-log-exporter version %s", version)
	log.Printf("Listen address: %s", *listenAddr)
	log.Printf("Metrics path: %s", *metricsPath)
	log.Printf("Log file: %s", *logFile)
	log.Printf("Config file: %s", *configFile)
	log.Printf("Position file: %s", *positionFile)
	log.Printf("Parse interval: %s", *interval)

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Printf("Warning: failed to load config: %v, using defaults", err)
		cfg = getDefaultConfig()
	} else {
		log.Printf("Configuration loaded:")
		log.Printf("  Log format: %s", cfg.LogFormat.Type)
		log.Printf("  Duration unit: %s", cfg.LogFormat.DurationUnit)
		log.Printf("  Track all domains: %v", cfg.Global.TrackAllDomains)
		log.Printf("  Max domains: %d", cfg.Global.MaxDomains)
		log.Printf("  Monitored domains: %d", len(cfg.MonitoredDomains))
		log.Printf("  Domain patterns: %d", len(cfg.DomainPatterns))

		// Show custom labels that will be used
		customLabels := cfg.GetCustomLabelKeys()
		if len(customLabels) > 0 {
			log.Printf("  Custom labels: %v", customLabels)
		}
	}

	// Initialize metrics with custom label keys
	customLabelKeys := cfg.GetCustomLabelKeys()
	m := metrics.NewMetrics(customLabelKeys)

	// Initialize parser
	p := parser.NewParser(*logFile, *positionFile, m, cfg)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle(*metricsPath, promhttp.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
<head><title>Squid Log Exporter</title></head>
<body>
<h1>Squid Log Exporter</h1>
<p><a href='` + *metricsPath + `'>Metrics</a></p>
<p>Version: ` + version + `</p>
</body>
</html>`))
	})

	server := &http.Server{
		Addr:    *listenAddr,
		Handler: mux,
	}

	// Start HTTP server in goroutine
	go func() {
		log.Printf("Starting HTTP server on %s", *listenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Parse immediately on startup
	go func() {
		log.Println("Initial log parsing...")
		if err := p.Parse(); err != nil {
			log.Printf("Error parsing log: %v", err)
		}
	}()

	// Periodic parsing ticker
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	// Main loop
	log.Println("Squid log exporter started successfully")

	for {
		select {
		case <-ticker.C:
			log.Println("Parsing log file...")
			if err := p.Parse(); err != nil {
				log.Printf("Error parsing log: %v", err)
			}

		case sig := <-sigChan:
			log.Printf("Received shutdown signal: %s", sig)

			// Stop ticker
			ticker.Stop()

			// Shutdown HTTP server with timeout
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			log.Println("Shutting down HTTP server...")
			if err := server.Shutdown(shutdownCtx); err != nil {
				log.Printf("HTTP server shutdown error: %v", err)
			}

			// Final parse to save last position
			log.Println("Final log parse before shutdown...")
			if err := p.Parse(); err != nil {
				log.Printf("Error in final parse: %v", err)
			}

			log.Println("Shutdown complete")
			return
		}
	}
}

func getDefaultConfig() *config.Config {
	return &config.Config{
		Global: config.GlobalConfig{
			TrackAllDomains: true,
			MaxDomains:      10000,
		},
		LogFormat: config.LogFormatConfig{
			Type: "squid_native",
			Fields: map[string]int{
				"timestamp":    0,
				"duration":     1,
				"client_ip":    2,
				"result_code":  3,
				"bytes":        4,
				"method":       5,
				"url":          6,
				"rfc931":       7,
				"hierarchy":    8,
				"content_type": 9,
			},
			TimestampFormat: "unix",
			DurationUnit:    "ms",
		},
	}
}
