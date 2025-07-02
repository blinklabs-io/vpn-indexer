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

package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/ca"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
)

const (
	healthcheckPath = "/healthcheck"
)

type Api struct {
	cfg *config.Config
	db  *database.Database
	ca  *ca.Ca
}

var api *Api

func Start(cfg *config.Config, db *database.Database, ca *ca.Ca) error {
	logger := slog.Default()
	logger.Info("initializing API server")

	api = &Api{
		cfg: cfg,
		db:  db,
		ca:  ca,
	}

	//
	// Main HTTP server for API endpoints
	//
	mainMux := http.NewServeMux()

	// Healthcheck
	mainMux.HandleFunc(healthcheckPath, api.handleHealthcheck)

	// API routes
	mainMux.HandleFunc("/api/client/list", api.handleClientList)
	mainMux.HandleFunc("/api/client/profile", api.handleClientProfile)
	mainMux.HandleFunc("/api/client/available", api.handleClientAvailable)

	// Wrap the mainMux with an access-logging middleware
	mainHandler := api.logMiddleware(mainMux, logger)

	// Start API server
	logger.Info("starting API listener",
		"address", cfg.Api.ListenAddress,
		"port", cfg.Api.ListenPort,
	)
	server := &http.Server{
		Addr: fmt.Sprintf(
			"%s:%d",
			cfg.Api.ListenAddress,
			cfg.Api.ListenPort,
		),
		Handler:           mainHandler,
		ReadHeaderTimeout: 60 * time.Second,
	}
	err := server.ListenAndServe()
	return err
}

func (a *Api) logMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrap the ResponseWriter to capture status code
		rec := &statusRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}
		next.ServeHTTP(rec, r)

		// Skip logging on a healthcheck request if healthcheck logging is disabled
		if !a.cfg.Api.LogHealthcheck {
			if r.URL.Path == healthcheckPath {
				return
			}
		}

		logger.Info("handled request",
			"status", rec.statusCode,
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
		)
	})
}

// statusRecorder helps to record the response status code
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// handleHealthcheck responds to GET /healthcheck
func (*Api) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"healthy": true}`))
}
