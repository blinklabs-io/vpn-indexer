// Copyright 2026 Blink Labs Software
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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
)

// TestConcurrentAllocateIP exercises the production database.New() code
// path (not the in-memory test helper) with many goroutines hammering
// AllocateIP concurrently, the way the HTTP API, the WireGuard peer expiry
// ticker, and the chain indexer all can in practice. Before the fix, this
// reliably produced either "database is locked (SQLITE_BUSY)" errors or,
// worse, silently handed out duplicate IPs (since SQLite ignores the
// "SELECT ... FOR UPDATE" hint AllocateIP relies on).
func TestConcurrentAllocateIP(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Directory: t.TempDir(),
		},
		Vpn: config.VpnConfig{
			Region: "test",
		},
	}
	db, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	const goroutines = 25
	var wg sync.WaitGroup
	ips := make([]string, goroutines)
	errs := make([]error, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ip, err := db.AllocateIP("test")
			ips[i] = ip
			errs[i] = err
		}(i)
	}
	wg.Wait()

	seen := make(map[string]int)
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error allocating IP: %v", i, err)
			continue
		}
		seen[ips[i]]++
	}
	for ip, count := range seen {
		if count > 1 {
			t.Errorf("IP %s was allocated %d times, expected 1", ip, count)
		}
	}
	if len(seen) != goroutines {
		t.Errorf(
			"expected %d unique IPs allocated, got %d (%v)",
			goroutines,
			len(seen),
			ips,
		)
	}
}

// TestConcurrentMixedWrites exercises concurrent writers across several
// different tables/methods at once (mirroring the indexer, API, and
// WireGuard manager writing to the DB from separate goroutines
// simultaneously) to make sure none of them surface SQLITE_BUSY errors.
func TestConcurrentMixedWrites(t *testing.T) {
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Directory: t.TempDir(),
		},
		Vpn: config.VpnConfig{
			Region: "test",
		},
	}
	db, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	const iterations = 25
	expiration := time.Now().Add(24 * time.Hour)
	var wg sync.WaitGroup
	errCh := make(chan error, iterations*3)

	for i := range iterations {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			assetName := []byte(fmt.Sprintf("asset-%d", i))
			if err := db.AddClient(
				assetName,
				expiration,
				[]byte("cred"),
				"test",
				[]byte("txhash"),
				0,
			); err != nil {
				errCh <- fmt.Errorf("AddClient: %w", err)
			}
		}(i)

		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pubkey := fmt.Sprintf("pubkey-%d", i)
			ip, err := db.AllocateIP("test")
			if err != nil {
				errCh <- fmt.Errorf("AllocateIP: %w", err)
				return
			}
			if err := db.AddWGPeer(
				[]byte(fmt.Sprintf("asset-%d", i)),
				pubkey,
				ip,
			); err != nil {
				errCh <- fmt.Errorf("AddWGPeer: %w", err)
			}
		}(i)

		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := db.HasWGPeers(); err != nil {
				errCh <- fmt.Errorf("HasWGPeers: %w", err)
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("unexpected concurrent write error: %v", err)
	}
}
