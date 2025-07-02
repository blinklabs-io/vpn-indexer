// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof" // #nosec G108
	"os"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/api"
	"github.com/blinklabs-io/vpn-indexer/internal/ca"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/crl"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
	"github.com/blinklabs-io/vpn-indexer/internal/indexer"
	"github.com/blinklabs-io/vpn-indexer/internal/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/automaxprocs/maxprocs"
)

var cmdlineFlags struct {
	configFile string
}

func slogPrintf(format string, v ...any) {
	slog.Info(fmt.Sprintf(format, v...))
}

func main() {
	flag.StringVar(
		&cmdlineFlags.configFile,
		"config",
		"",
		"path to config file to load",
	)
	flag.Parse()

	// Load config
	cfg, err := config.Load(cmdlineFlags.configFile)
	if err != nil {
		fmt.Printf("Failed to load config: %s\n", err)
		os.Exit(1)
	}

	// Configure logger
	var level slog.Level
	if cfg.Logging.Debug {
		level = slog.LevelDebug
	} else {
		level = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Open database
	db, err := database.New(cfg, logger)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}

	// Configure max processes with our logger wrapper, toss undo func
	_, err = maxprocs.Set(maxprocs.Logger(slogPrintf))
	if err != nil {
		// If we hit this, something really wrong happened
		logger.Error(err.Error())
		os.Exit(1)
	}

	slog.Info(
		fmt.Sprintf("vpn-indexer %s started", version.GetVersionString()),
	)

	// Start debug listener
	if cfg.Debug.ListenPort > 0 {
		slog.Info(
			fmt.Sprintf(
				"starting debug listener on %s:%d",
				cfg.Debug.ListenAddress,
				cfg.Debug.ListenPort,
			),
		)
		go func() {
			debugger := &http.Server{
				Addr: fmt.Sprintf(
					"%s:%d",
					cfg.Debug.ListenAddress,
					cfg.Debug.ListenPort,
				),
				ReadHeaderTimeout: 60 * time.Second,
			}
			err := debugger.ListenAndServe()
			if err != nil {
				slog.Error(
					fmt.Sprintf("failed to start debug listener: %s", err),
				)
				os.Exit(1)
			}
		}()
	}

	// Start metrics listener
	if cfg.Metrics.ListenPort > 0 {
		metricsListenAddr := fmt.Sprintf(
			"%s:%d",
			cfg.Metrics.ListenAddress,
			cfg.Metrics.ListenPort,
		)
		slog.Info(
			"starting listener for prometheus metrics connections on " + metricsListenAddr,
		)
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsSrv := &http.Server{
			Addr:         metricsListenAddr,
			WriteTimeout: 10 * time.Second,
			ReadTimeout:  10 * time.Second,
			Handler:      metricsMux,
		}
		go func() {
			if err := metricsSrv.ListenAndServe(); err != nil {
				slog.Error(
					fmt.Sprintf("failed to start metrics listener: %s", err),
				)
				os.Exit(1)
			}
		}()
	}

	// Configure CA
	ca, err := ca.New(cfg)
	if err != nil {
		slog.Error(
			fmt.Sprintf("failed to configure CA: %s", err),
		)
		os.Exit(1)
	}

	// Configure CRL
	_, err = crl.New(cfg, logger, db, ca)
	if err != nil {
		slog.Error(
			fmt.Sprintf("failed to configure CRL: %s", err),
		)
		os.Exit(1)
	}

	// Start indexer
	if err := indexer.GetIndexer().Start(cfg, logger, db, ca); err != nil {
		slog.Error(
			fmt.Sprintf("failed to start indexer: %s", err),
		)
		os.Exit(1)
	}

	// Start API listener
	if err := api.Start(cfg, db, ca); err != nil {
		slog.Error(
			"failed to start API:",
			"error",
			err,
		)
		os.Exit(1)
	}

	// Wait forever
	select {}
}
