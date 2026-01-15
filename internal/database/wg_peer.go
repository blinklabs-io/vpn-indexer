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
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrIPPoolExhausted is returned when no more IPs are available in the pool
var ErrIPPoolExhausted = errors.New("IP pool exhausted: no available addresses")

// WGPeer tracks WireGuard device registrations (cache of S3 data).
// The database serves as a local cache; S3 is the source of truth.
type WGPeer struct {
	ID         uint      `gorm:"primaryKey"`
	AssetName  []byte    `gorm:"index;not null"`       // Links to Client.AssetName
	Pubkey     string    `gorm:"uniqueIndex;not null"` // WireGuard public key (base64)
	AssignedIP string    `gorm:"not null"`             // e.g., "10.8.0.42"
	CreatedAt  time.Time `gorm:"autoCreateTime"`
}

func (WGPeer) TableName() string {
	return "wg_peer"
}

// WGIPPool tracks IP allocation state per region
type WGIPPool struct {
	Region string `gorm:"primaryKey"`
	NextIP int    `gorm:"not null;default:2"` // Next IP octet to assign (10.8.0.X)
}

func (WGIPPool) TableName() string {
	return "wg_ip_pool"
}

// AddWGPeer adds a new WireGuard peer to the database
func (d *Database) AddWGPeer(
	assetName []byte,
	pubkey string,
	assignedIP string,
) error {
	peer := WGPeer{
		AssetName:  assetName,
		Pubkey:     pubkey,
		AssignedIP: assignedIP,
	}
	if result := d.db.Create(&peer); result.Error != nil {
		return result.Error
	}
	return nil
}

// GetWGPeersByAsset returns all WireGuard peers for a given asset name
func (d *Database) GetWGPeersByAsset(assetName []byte) ([]WGPeer, error) {
	var peers []WGPeer
	result := d.db.Where("asset_name = ?", assetName).
		Order("created_at").
		Find(&peers)
	if result.Error != nil {
		return nil, result.Error
	}
	return peers, nil
}

// GetWGPeerByPubkey returns a WireGuard peer by its public key
func (d *Database) GetWGPeerByPubkey(pubkey string) (*WGPeer, error) {
	var peer WGPeer
	result := d.db.Where("pubkey = ?", pubkey).First(&peer)
	if result.Error != nil {
		return nil, result.Error
	}
	return &peer, nil
}

// DeleteWGPeer removes a WireGuard peer by its public key.
// Note: Callers should also call DeallocateIP to release the peer's IP back
// to the pool.
func (d *Database) DeleteWGPeer(pubkey string) error {
	result := d.db.Where("pubkey = ?", pubkey).Delete(&WGPeer{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// CountWGPeersByAsset returns the number of WireGuard peers for a given asset
func (d *Database) CountWGPeersByAsset(assetName []byte) (int64, error) {
	var count int64
	result := d.db.Model(&WGPeer{}).
		Where("asset_name = ?", assetName).
		Count(&count)
	if result.Error != nil {
		return 0, result.Error
	}
	return count, nil
}

// AllocateIP atomically allocates the next available IP address for a region.
// Returns IP in format "{subnet}.X" where X is between 2 and 254.
// Skips .0 (network), .1 (gateway), and .255 (broadcast).
// Returns ErrIPPoolExhausted if all IPs (2-254) are in use.
func (d *Database) AllocateIP(region string) (string, error) {
	var allocatedIP string

	subnet := d.config.Vpn.WGSubnet
	if subnet == "" {
		subnet = "10.8.0"
	}

	err := d.db.Transaction(func(tx *gorm.DB) error {
		// Get or create the IP pool for this region
		// Use SELECT ... FOR UPDATE to serialize concurrent allocations
		var pool WGIPPool
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("region = ?", region).
			First(&pool)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				// Create new pool starting at IP 2
				pool = WGIPPool{
					Region: region,
					NextIP: 2,
				}
				if err := tx.Create(&pool).Error; err != nil {
					// Handle race condition: another request may have created
					// the pool concurrently. Re-query to check if pool now exists.
					var existingPool WGIPPool
					reqErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
						Where("region = ?", region).
						First(&existingPool)
					if reqErr.Error != nil {
						// Pool still doesn't exist, return original create error
						return fmt.Errorf(
							"failed to create IP pool: %w (requery: %v)",
							err,
							reqErr.Error,
						)
					}
					// Pool was created by concurrent request, use it
					pool = existingPool
				}
			} else {
				return result.Error
			}
		}

		// Get all currently allocated IPs for this region for collision detection
		var allocatedIPs []string
		if err := tx.Model(&WGPeer{}).
			Joins("JOIN client ON wg_peer.asset_name = client.asset_name").
			Where("client.region = ?", region).
			Pluck("wg_peer.assigned_ip", &allocatedIPs).Error; err != nil {
			return fmt.Errorf("failed to get allocated IPs: %w", err)
		}

		// Build a set of used IP octets
		usedOctets := make(map[int]bool)
		for _, ip := range allocatedIPs {
			// Extract last octet from IP like "10.8.0.42"
			parts := strings.Split(ip, ".")
			if len(parts) == 4 {
				if octet, err := strconv.Atoi(parts[3]); err == nil {
					usedOctets[octet] = true
				}
			}
		}

		// Find next available IP, starting from pool.NextIP
		startIP := pool.NextIP
		currentIP := startIP
		found := false

		for {
			if !usedOctets[currentIP] {
				found = true
				break
			}

			// Move to next IP
			currentIP++
			if currentIP > 254 {
				currentIP = 2
			}

			// If we've wrapped around to start, pool is exhausted
			if currentIP == startIP {
				break
			}
		}

		if !found {
			return ErrIPPoolExhausted
		}

		// Calculate next IP for the pool (for next allocation attempt)
		nextIP := currentIP + 1
		if nextIP > 254 {
			nextIP = 2
		}

		// Update the pool with next IP hint
		pool.NextIP = nextIP
		if err := tx.Save(&pool).Error; err != nil {
			return err
		}

		// Format the allocated IP
		allocatedIP = fmt.Sprintf("%s.%d", subnet, currentIP)
		return nil
	})

	if err != nil {
		return "", err
	}

	return allocatedIP, nil
}

// GetExpiredWGPeers returns all WireGuard peers whose subscriptions have expired
func (d *Database) GetExpiredWGPeers() ([]WGPeer, error) {
	var peers []WGPeer
	now := time.Now()
	result := d.db.
		Joins("JOIN client ON wg_peer.asset_name = client.asset_name").
		Where(
			"client.expiration < ? AND client.region = ?",
			now,
			d.config.Vpn.Region,
		).
		Find(&peers)
	if result.Error != nil {
		return nil, result.Error
	}
	return peers, nil
}

// HasWGPeers checks if there are any WireGuard peers in the database
func (d *Database) HasWGPeers() (bool, error) {
	var count int64
	result := d.db.Model(&WGPeer{}).Count(&count)
	if result.Error != nil {
		return false, result.Error
	}
	return count > 0, nil
}

// RebuildIPPool rebuilds the IP pool for a region based on existing peer IPs
func (d *Database) RebuildIPPool(region string) error {
	// Get all assigned IPs for peers in this region
	var assignedIPs []string
	result := d.db.Model(&WGPeer{}).
		Joins("JOIN client ON wg_peer.asset_name = client.asset_name").
		Where("client.region = ?", region).
		Pluck("wg_peer.assigned_ip", &assignedIPs)
	if result.Error != nil {
		return result.Error
	}

	// Find max octet by parsing IPs properly
	maxOctet := 0
	for _, ip := range assignedIPs {
		parts := strings.Split(ip, ".")
		if len(parts) == 4 {
			if octet, err := strconv.Atoi(parts[3]); err == nil {
				if octet > maxOctet {
					maxOctet = octet
				}
			}
		}
	}

	// Set next IP to max + 1 (or 2 if no peers)
	nextIP := 2
	if maxOctet > 0 {
		nextIP = maxOctet + 1
		// Wrap around if needed
		if nextIP > 254 {
			nextIP = 2
		}
	}

	return d.db.Save(&WGIPPool{Region: region, NextIP: nextIP}).Error
}

// DeallocateIP releases an IP back to the pool by resetting NextIP to point
// to the deallocated IP's octet. This ensures the IP is immediately available
// for the next allocation attempt. Use this when an IP was allocated but the
// peer was not successfully persisted (e.g., S3 save failed).
func (d *Database) DeallocateIP(region, ip string) error {
	// Extract the last octet from the IP
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return fmt.Errorf("invalid IP format: %s", ip)
	}
	octet, err := strconv.Atoi(parts[3])
	if err != nil {
		return fmt.Errorf("invalid IP octet: %s", parts[3])
	}
	if octet < 2 || octet > 254 {
		return fmt.Errorf(
			"IP octet %d out of valid range (2-254): reserved address",
			octet,
		)
	}

	// Update the pool's NextIP to point to the deallocated IP
	// so it's the next one tried on allocation
	return d.db.Model(&WGIPPool{}).
		Where("region = ?", region).
		Update("next_ip", octet).Error
}

// GetActivePeersForRegion returns all WireGuard peers for active (non-expired)
// subscriptions in the specified region
func (d *Database) GetActivePeersForRegion(region string) ([]WGPeer, error) {
	var peers []WGPeer
	now := time.Now()
	result := d.db.
		Joins("JOIN client ON wg_peer.asset_name = client.asset_name").
		Where(
			"client.region = ? AND client.expiration > ?",
			region,
			now,
		).
		Find(&peers)
	if result.Error != nil {
		return nil, result.Error
	}
	return peers, nil
}
