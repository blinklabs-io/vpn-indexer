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
	// WAL journal mode. Combined with synchronous=NORMAL (safe under WAL: an
	// OS crash can lose the most recent commits but never corrupts the DB),
	// this avoids an fsync on every single write. That matters a lot during
	// initial chain sync, where we persist a chainsync cursor update for
	// every block processed (see AddCursorPoint) - with the default
	// synchronous=FULL, each of those was a blocking disk sync.
	//
	// Also set a busy timeout (beyond the driver's own 5s default) so that
	// any brief lock contention from an external process (e.g. an sqlite3
	// shell or a backup job opening the same file) causes SQLite to retry
	// internally rather than immediately failing with SQLITE_BUSY
	// ("database is locked"). Kept well under typical HTTP client/proxy
	// timeouts so a locked DB still fails an API request in bounded time
	// rather than hanging it.
	connOpts := "_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(10000)"
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
	// SQLite only allows a single writer at a time. gorm/database-sql will
	// happily hand out multiple concurrent connections from Go's
	// connection pool, and several parts of this app write to the DB from
	// separate goroutines (chain indexer, HTTP API handlers, WireGuard
	// peer expiry ticker). A transaction that reads a row and later writes
	// it (see AllocateIP) can't rely on SQLite row/table locking to
	// serialize those writers, since SQLite has no such thing (its
	// gorm dialector silently drops "SELECT ... FOR UPDATE" clauses). Left
	// unbounded, concurrent writers race and can surface as either
	// "database is locked (SQLITE_BUSY)" errors (if contention outlasts
	// busy_timeout) or as silent data races (e.g. two requests computing
	// the same "next" IP address). Limiting the pool to a single
	// connection forces Go's database/sql to serialize all access itself,
	// which removes the internal contention entirely; only pragma
	// busy_timeout is left to help beyond that against outside processes,
	// like a `sqlite3` shell or backup script, momentarily opening the
	// same file.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get underlying sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
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
