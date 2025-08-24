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

package database

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var ErrRecordNotFound = gorm.ErrRecordNotFound

type Database struct {
	config *config.Config
	db     *gorm.DB
	logger *slog.Logger
}

func New(cfg *config.Config, logger *slog.Logger) (*Database, error) {
	if logger == nil {
		// Create logger to throw away logs
		// We do this so we don't have to add guards around every log operation
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	dataDir := cfg.Database.Directory
	// Make sure that we can read data dir, and create if it doesn't exist
	if _, err := os.Stat(dataDir); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("failed to read data dir: %w", err)
		}
		// Create data directory
		if err := os.MkdirAll(dataDir, fs.ModePerm); err != nil {
			return nil, fmt.Errorf("failed to create data dir: %w", err)
		}
	}
	// Open sqlite DB
	dbPath := filepath.Join(
		dataDir,
		"vpn-indexer.sqlite",
	)
	// WAL journal mode
	connOpts := "_pragma=journal_mode(WAL)"
	db, err := gorm.Open(
		sqlite.Open(
			fmt.Sprintf("file:%s?%s", dbPath, connOpts),
		),
		&gorm.Config{
			Logger: gormlogger.Discard,
		},
	)
	if err != nil {
		return nil, err
	}
	d := &Database{
		config: cfg,
		db:     db,
		logger: logger,
	}
	// Create table schemas
	for _, model := range MigrateModels {
		if err := d.db.AutoMigrate(model); err != nil {
			return nil, err
		}
	}
	return d, nil
}
