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
	"fmt"
	"testing"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newTestDatabase creates an in-memory SQLite database for testing
func newTestDatabase(t *testing.T) *Database {
	t.Helper()

	cfg := &config.Config{
		Vpn: config.VpnConfig{
			Region: "test",
		},
	}

	// Open in-memory SQLite database with shared cache to ensure migrations
	// and queries run against the same in-memory instance. Use unique name
	// per test to maintain test isolation.
	dbURI := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(
		sqlite.Open(dbURI),
		&gorm.Config{
			Logger: gormlogger.Discard,
		},
	)
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	// Disable connection pooling to prevent isolated connections
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	d := &Database{
		config: cfg,
		db:     db,
	}

	// Run migrations for WGPeer, WGIPPool, and Client
	if err := db.AutoMigrate(&WGPeer{}, &WGIPPool{}, &Client{}); err != nil {
		t.Fatalf("failed to migrate tables: %v", err)
	}

	return d
}

func TestAddWGPeer(t *testing.T) {
	db := newTestDatabase(t)

	assetName := []byte("test-asset-123")
	pubkey := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijk="
	assignedIP := "10.8.0.42"

	err := db.AddWGPeer(assetName, pubkey, assignedIP)
	if err != nil {
		t.Fatalf("unexpected error adding WG peer: %v", err)
	}

	// Verify peer was added
	peers, err := db.GetWGPeersByAsset(assetName)
	if err != nil {
		t.Fatalf("unexpected error getting WG peers: %v", err)
	}

	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}

	if string(peers[0].AssetName) != string(assetName) {
		t.Fatalf(
			"expected asset name %q, got %q",
			string(assetName),
			string(peers[0].AssetName),
		)
	}

	if peers[0].Pubkey != pubkey {
		t.Fatalf("expected pubkey %q, got %q", pubkey, peers[0].Pubkey)
	}

	if peers[0].AssignedIP != assignedIP {
		t.Fatalf(
			"expected assigned IP %q, got %q",
			assignedIP,
			peers[0].AssignedIP,
		)
	}
}

func TestAddWGPeerDuplicatePubkey(t *testing.T) {
	db := newTestDatabase(t)

	assetName := []byte("test-asset-123")
	pubkey := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijk="
	assignedIP := "10.8.0.42"

	err := db.AddWGPeer(assetName, pubkey, assignedIP)
	if err != nil {
		t.Fatalf("unexpected error adding first WG peer: %v", err)
	}

	// Try to add duplicate pubkey
	err = db.AddWGPeer(assetName, pubkey, "10.8.0.43")
	if err == nil {
		t.Fatal("expected error for duplicate pubkey, got nil")
	}
}

func TestGetWGPeersByAsset(t *testing.T) {
	db := newTestDatabase(t)

	assetName1 := []byte("asset-1")
	assetName2 := []byte("asset-2")

	// Add peers for asset 1
	if err := db.AddWGPeer(assetName1, "pubkey1", "10.8.0.2"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}
	if err := db.AddWGPeer(assetName1, "pubkey2", "10.8.0.3"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	// Add peer for asset 2
	if err := db.AddWGPeer(assetName2, "pubkey3", "10.8.0.4"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	// Get peers for asset 1
	peers, err := db.GetWGPeersByAsset(assetName1)
	if err != nil {
		t.Fatalf("unexpected error getting WG peers: %v", err)
	}

	if len(peers) != 2 {
		t.Fatalf("expected 2 peers for asset 1, got %d", len(peers))
	}

	// Get peers for asset 2
	peers, err = db.GetWGPeersByAsset(assetName2)
	if err != nil {
		t.Fatalf("unexpected error getting WG peers: %v", err)
	}

	if len(peers) != 1 {
		t.Fatalf("expected 1 peer for asset 2, got %d", len(peers))
	}
}

func TestGetWGPeersByAssetEmpty(t *testing.T) {
	db := newTestDatabase(t)

	assetName := []byte("nonexistent-asset")

	peers, err := db.GetWGPeersByAsset(assetName)
	if err != nil {
		t.Fatalf("unexpected error getting WG peers: %v", err)
	}

	if len(peers) != 0 {
		t.Fatalf("expected 0 peers for nonexistent asset, got %d", len(peers))
	}
}

func TestGetWGPeerByPubkey(t *testing.T) {
	db := newTestDatabase(t)

	assetName := []byte("test-asset")
	pubkey := "test-pubkey-123"
	assignedIP := "10.8.0.42"

	if err := db.AddWGPeer(assetName, pubkey, assignedIP); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	peer, err := db.GetWGPeerByPubkey(pubkey)
	if err != nil {
		t.Fatalf("unexpected error getting WG peer by pubkey: %v", err)
	}

	if peer == nil {
		t.Fatal("expected peer to not be nil")
	}

	if peer.Pubkey != pubkey {
		t.Fatalf("expected pubkey %q, got %q", pubkey, peer.Pubkey)
	}
}

func TestGetWGPeerByPubkeyNotFound(t *testing.T) {
	db := newTestDatabase(t)

	_, err := db.GetWGPeerByPubkey("nonexistent-pubkey")
	if err == nil {
		t.Fatal("expected error for nonexistent pubkey, got nil")
	}
}

func TestDeleteWGPeer(t *testing.T) {
	db := newTestDatabase(t)

	assetName := []byte("test-asset")
	pubkey := "test-pubkey-123"
	assignedIP := "10.8.0.42"

	if err := db.AddWGPeer(assetName, pubkey, assignedIP); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	// Delete the peer
	err := db.DeleteWGPeer(pubkey)
	if err != nil {
		t.Fatalf("unexpected error deleting WG peer: %v", err)
	}

	// Verify peer was deleted
	_, err = db.GetWGPeerByPubkey(pubkey)
	if err == nil {
		t.Fatal("expected error for deleted peer, got nil")
	}
}

func TestCountWGPeersByAsset(t *testing.T) {
	db := newTestDatabase(t)

	assetName := []byte("test-asset")

	// Add 3 peers
	if err := db.AddWGPeer(assetName, "pubkey1", "10.8.0.2"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}
	if err := db.AddWGPeer(assetName, "pubkey2", "10.8.0.3"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}
	if err := db.AddWGPeer(assetName, "pubkey3", "10.8.0.4"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	count, err := db.CountWGPeersByAsset(assetName)
	if err != nil {
		t.Fatalf("unexpected error counting WG peers: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestCountWGPeersByAssetEmpty(t *testing.T) {
	db := newTestDatabase(t)

	assetName := []byte("nonexistent-asset")

	count, err := db.CountWGPeersByAsset(assetName)
	if err != nil {
		t.Fatalf("unexpected error counting WG peers: %v", err)
	}

	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}

func TestAllocateIP(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// First allocation should return 10.8.0.2
	ip, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP: %v", err)
	}
	if ip != "10.8.0.2" {
		t.Fatalf("expected first IP to be 10.8.0.2, got %s", ip)
	}

	// Second allocation should return 10.8.0.3
	ip, err = db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP: %v", err)
	}
	if ip != "10.8.0.3" {
		t.Fatalf("expected second IP to be 10.8.0.3, got %s", ip)
	}

	// Third allocation should return 10.8.0.4
	ip, err = db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP: %v", err)
	}
	if ip != "10.8.0.4" {
		t.Fatalf("expected third IP to be 10.8.0.4, got %s", ip)
	}
}

func TestAllocateIPSequential(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Allocate multiple IPs and verify they are sequential
	expectedPrefix := "10.8.0."
	for i := 2; i <= 10; i++ {
		ip, err := db.AllocateIP(region)
		if err != nil {
			t.Fatalf("unexpected error allocating IP %d: %v", i, err)
		}
		// Verify IP has correct prefix
		if len(ip) < len(expectedPrefix) ||
			ip[:len(expectedPrefix)] != expectedPrefix {
			t.Fatalf("expected IP prefix %q, got %q", expectedPrefix, ip)
		}
		// Verify IP octet is sequential
		expectedIP := fmt.Sprintf("10.8.0.%d", i)
		if ip != expectedIP {
			t.Fatalf("expected IP %q, got %q (not sequential)", expectedIP, ip)
		}
	}
}

func TestAllocateIPWrapAt254(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Create client record for the peer (required for region-filtered IP lookup)
	if err := db.db.Create(&Client{AssetName: []byte("asset"), Region: region}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}

	// Manually set the pool to 254
	pool := WGIPPool{
		Region: region,
		NextIP: 254,
	}
	if err := db.db.Create(&pool).Error; err != nil {
		t.Fatalf("failed to create WGIPPool in setup: %v", err)
	}

	// First allocation should return 10.8.0.254
	ip, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP: %v", err)
	}
	if ip != "10.8.0.254" {
		t.Fatalf("expected IP to be 10.8.0.254, got %s", ip)
	}

	// Add this IP to the peer table to simulate it being used
	if err := db.AddWGPeer([]byte("asset"), "pubkey254", ip); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	// Next allocation should wrap to 10.8.0.2 (skipping 254 which is now used)
	ip, err = db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP: %v", err)
	}
	if ip != "10.8.0.2" {
		t.Fatalf("expected IP to wrap to 10.8.0.2, got %s", ip)
	}
}

func TestAllocateIPSkipsUsedIPs(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Create client records for the peers (required for region-filtered IP lookup)
	if err := db.db.Create(&Client{AssetName: []byte("asset1"), Region: region}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}
	if err := db.db.Create(&Client{AssetName: []byte("asset2"), Region: region}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}
	if err := db.db.Create(&Client{AssetName: []byte("asset3"), Region: region}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}

	// Add peers with IPs 2, 3, 4
	if err := db.AddWGPeer([]byte("asset1"), "pubkey1", "10.8.0.2"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}
	if err := db.AddWGPeer([]byte("asset2"), "pubkey2", "10.8.0.3"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}
	if err := db.AddWGPeer([]byte("asset3"), "pubkey3", "10.8.0.4"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	// Allocate should skip 2, 3, 4 and return 5
	ip, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP: %v", err)
	}
	if ip != "10.8.0.5" {
		t.Fatalf("expected IP to be 10.8.0.5 (skipping used IPs), got %s", ip)
	}
}

func TestAllocateIPPoolExhausted(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Create client record for the peers (required for region-filtered IP lookup)
	if err := db.db.Create(&Client{AssetName: []byte("asset"), Region: region}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}

	// Fill the entire IP range (2-254 = 253 IPs)
	for i := 2; i <= 254; i++ {
		ip := fmt.Sprintf("10.8.0.%d", i)
		pubkey := fmt.Sprintf("pubkey%d", i)
		if err := db.AddWGPeer([]byte("asset"), pubkey, ip); err != nil {
			t.Fatalf("failed to add WG peer %d in setup: %v", i, err)
		}
	}

	// Next allocation should fail with ErrIPPoolExhausted
	_, err := db.AllocateIP(region)
	if err == nil {
		t.Fatal("expected error when IP pool is exhausted, got nil")
	}
	if err != ErrIPPoolExhausted {
		t.Fatalf("expected ErrIPPoolExhausted, got %v", err)
	}
}

func TestAllocateIPMultipleRegions(t *testing.T) {
	db := newTestDatabase(t)

	region1 := "region-1"
	region2 := "region-2"

	// Allocate IPs for region 1
	ip1, err := db.AllocateIP(region1)
	if err != nil {
		t.Fatalf("unexpected error allocating IP for region 1: %v", err)
	}
	if ip1 != "10.8.0.2" {
		t.Fatalf("expected first IP for region 1 to be 10.8.0.2, got %s", ip1)
	}

	// Allocate IPs for region 2 - should start fresh at 2
	ip2, err := db.AllocateIP(region2)
	if err != nil {
		t.Fatalf("unexpected error allocating IP for region 2: %v", err)
	}
	if ip2 != "10.8.0.2" {
		t.Fatalf("expected first IP for region 2 to be 10.8.0.2, got %s", ip2)
	}

	// Second allocation for region 1 should continue from 3
	ip1, err = db.AllocateIP(region1)
	if err != nil {
		t.Fatalf("unexpected error allocating second IP for region 1: %v", err)
	}
	if ip1 != "10.8.0.3" {
		t.Fatalf("expected second IP for region 1 to be 10.8.0.3, got %s", ip1)
	}
}

func TestHasWGPeers(t *testing.T) {
	db := newTestDatabase(t)

	// Initially should have no peers
	hasPeers, err := db.HasWGPeers()
	if err != nil {
		t.Fatalf("unexpected error checking for WG peers: %v", err)
	}
	if hasPeers {
		t.Fatal("expected no WG peers initially")
	}

	// Add a peer
	if err := db.AddWGPeer([]byte("asset"), "pubkey", "10.8.0.2"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	// Now should have peers
	hasPeers, err = db.HasWGPeers()
	if err != nil {
		t.Fatalf("unexpected error checking for WG peers: %v", err)
	}
	if !hasPeers {
		t.Fatal("expected WG peers after adding one")
	}
}

func TestRebuildIPPool(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Create client records for the peers (required for RebuildIPPool join)
	if err := db.db.Create(&Client{
		AssetName: []byte("asset1"),
		Region:    region,
	}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}
	if err := db.db.Create(&Client{
		AssetName: []byte("asset2"),
		Region:    region,
	}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}
	if err := db.db.Create(&Client{
		AssetName: []byte("asset3"),
		Region:    region,
	}).Error; err != nil {
		t.Fatalf("failed to create client in setup: %v", err)
	}

	// Add some peers with various IPs
	if err := db.AddWGPeer([]byte("asset1"), "pubkey1", "10.8.0.5"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}
	if err := db.AddWGPeer([]byte("asset2"), "pubkey2", "10.8.0.10"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}
	if err := db.AddWGPeer([]byte("asset3"), "pubkey3", "10.8.0.3"); err != nil {
		t.Fatalf("failed to add WG peer in setup: %v", err)
	}

	// Rebuild IP pool
	err := db.RebuildIPPool(region)
	if err != nil {
		t.Fatalf("unexpected error rebuilding IP pool: %v", err)
	}

	// Next allocation should be 11 (max was 10)
	ip, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP after rebuild: %v", err)
	}
	if ip != "10.8.0.11" {
		t.Fatalf("expected IP to be 10.8.0.11 after rebuild, got %s", ip)
	}
}

func TestRebuildIPPoolEmpty(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Rebuild IP pool with no peers
	err := db.RebuildIPPool(region)
	if err != nil {
		t.Fatalf("unexpected error rebuilding IP pool: %v", err)
	}

	// First allocation should be 2
	ip, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP after rebuild: %v", err)
	}
	if ip != "10.8.0.2" {
		t.Fatalf("expected IP to be 10.8.0.2 after rebuild, got %s", ip)
	}
}

func TestDeallocateIP(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Allocate first IP (should be 10.8.0.2)
	ip1, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating IP: %v", err)
	}
	if ip1 != "10.8.0.2" {
		t.Fatalf("expected first IP to be 10.8.0.2, got %s", ip1)
	}

	// Allocate second IP (should be 10.8.0.3)
	ip2, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating second IP: %v", err)
	}
	if ip2 != "10.8.0.3" {
		t.Fatalf("expected second IP to be 10.8.0.3, got %s", ip2)
	}

	// Deallocate the second IP (simulating S3 save failure)
	err = db.DeallocateIP(region, ip2)
	if err != nil {
		t.Fatalf("unexpected error deallocating IP: %v", err)
	}

	// Next allocation should be 10.8.0.3 again (the deallocated IP)
	ip3, err := db.AllocateIP(region)
	if err != nil {
		t.Fatalf("unexpected error allocating after dealloc: %v", err)
	}
	if ip3 != "10.8.0.3" {
		t.Fatalf("expected IP after dealloc to be 10.8.0.3, got %s", ip3)
	}
}

func TestDeallocateIPInvalidFormat(t *testing.T) {
	db := newTestDatabase(t)

	region := "test-region"

	// Test invalid IP format
	err := db.DeallocateIP(region, "invalid")
	if err == nil {
		t.Fatal("expected error for invalid IP format")
	}

	// Test invalid octet
	err = db.DeallocateIP(region, "10.8.0.abc")
	if err == nil {
		t.Fatal("expected error for invalid octet")
	}
}
