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
	"github.com/blinklabs-io/vpn-indexer/internal/client"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/crl"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
	"github.com/blinklabs-io/vpn-indexer/internal/indexer"
	"github.com/blinklabs-io/vpn-indexer/internal/jwt"
	"github.com/blinklabs-io/vpn-indexer/internal/version"
	"github.com/blinklabs-io/vpn-indexer/internal/wireguard"
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
		fmt.Sprintf(
			"vpn-indexer %s started for region %s",
			version.GetVersionString(),
			cfg.Vpn.Region,
		),
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
	crlInstance, err := crl.New(cfg, logger, db, ca)
	if err != nil {
		slog.Error(
			fmt.Sprintf("failed to configure CRL: %s", err),
		)
		os.Exit(1)
	}

	// Initialize WireGuard components if protocol is wireguard
	var wgClient *wireguard.Client
	var s3Client *client.Client
	if cfg.Vpn.Protocol == "wireguard" {
		slog.Info("initializing WireGuard components")

		// Initialize JWT issuer for WG container authentication
		jwtIssuer, err := jwt.NewIssuer(cfg.Vpn.WGJWTKeyFile)
		if err != nil {
			slog.Error(
				fmt.Sprintf("failed to initialize JWT issuer: %s", err),
			)
			os.Exit(1)
		}

		// Initialize WG container client
		wgClient = wireguard.NewClient(cfg.Vpn.WGContainerURL, jwtIssuer)

		// Health check WG container (warn but don't fail if not available)
		if err := wgClient.Health(); err != nil {
			slog.Warn(
				fmt.Sprintf(
					"WG container not available at startup: %s",
					err,
				),
			)
		} else {
			slog.Info("WG container health check passed")
		}

		// Initialize S3 client for peer registry
		s3Client = client.NewWithConfig(cfg)

		// Check if DB needs rebuild from S3
		hasData, err := db.HasWGPeers()
		if err != nil {
			slog.Warn(
				fmt.Sprintf("failed to check for WG peers in DB: %s", err),
			)
		} else if !hasData {
			slog.Info("no WG peers in DB, rebuilding from S3...")
			if err := s3Client.RebuildWGPeersFromS3(
				db,
				cfg.Vpn.Region,
			); err != nil {
				slog.Warn(
					fmt.Sprintf("failed to rebuild WG peers from S3: %s", err),
				)
			}
		}

		// Sync active peers to WG container
		slog.Info("syncing peers to WG container...")
		if err := wgClient.SyncPeersToContainer(db, cfg.Vpn.Region); err != nil {
			slog.Warn(
				fmt.Sprintf("failed to sync peers to WG container: %s", err),
			)
		}

		// Pass WG client and S3 client to CRL for cleanup operations
		crlInstance.SetWGClient(wgClient)
		crlInstance.SetS3Client(s3Client)
	}

	// Start indexer
	if err := indexer.GetIndexer().Start(cfg, logger, db, ca, crlInstance); err != nil {
		slog.Error(
			fmt.Sprintf("failed to start indexer: %s", err),
		)
		os.Exit(1)
	}

	// Start API listener
	if err := api.Start(cfg, db, ca, wgClient, s3Client); err != nil {
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
