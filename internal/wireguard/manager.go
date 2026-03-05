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

package wireguard

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/client"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
)

type Manager struct {
	config              *config.Config
	db                  *database.Database
	logger              *slog.Logger
	wgClient            *Client
	s3Client            *client.Client
	doneChan            chan struct{}
	stopOnce            sync.Once
	nextScheduledUpdate time.Time
}

func NewManager(
	cfg *config.Config,
	logger *slog.Logger,
	db *database.Database,
	wgClient *Client,
	s3Client *client.Client,
) (*Manager, error) {
	m := &Manager{
		config:   cfg,
		logger:   logger,
		db:       db,
		wgClient: wgClient,
		s3Client: s3Client,
		doneChan: make(chan struct{}),
	}
	if err := m.cleanupExpiredWGPeers(); err != nil {
		return nil, fmt.Errorf("expire clients: %w", err)
	}
	// Schedule automatic updates to expired peers
	m.scheduleUpdateExpiredPeers()
	return m, nil
}

func (m *Manager) Stop() {
	m.stopOnce.Do(func() {
		close(m.doneChan)
	})
}

func (m *Manager) scheduleUpdateExpiredPeers() {
	ticker := time.NewTicker(1 * time.Minute)
	m.nextScheduledUpdate = time.Now().Add(m.config.Vpn.WGExpireInterval)
	go func() {
		defer ticker.Stop()
		needsUpdate := false
		for {
			select {
			case <-m.doneChan:
				return
			case <-ticker.C:
				due := time.Now().After(m.nextScheduledUpdate)
				if due || needsUpdate {
					// Cleanup expired WireGuard peers
					if err := m.cleanupExpiredWGPeers(); err != nil {
						// Retry on next tick
						needsUpdate = true
						m.logger.Error(
							fmt.Sprintf(
								"failed to cleanup expired WG peers: %s",
								err,
							),
						)
						break
					}
					needsUpdate = false
					m.nextScheduledUpdate = m.nextScheduledUpdate.Add(
						m.config.Vpn.WGExpireInterval,
					)
				}
			}
		}
	}()
}

// cleanupExpiredWGPeers removes WireGuard peers for expired subscriptions.
// Checks doneChan between each peer to support graceful shutdown.
func (m *Manager) cleanupExpiredWGPeers() error {
	// Skip if not wireguard protocol
	if m.config.Vpn.Protocol != "wireguard" {
		return nil
	}

	// Get expired peers from DB
	expiredPeers, err := m.db.GetExpiredWGPeers()
	if err != nil {
		return fmt.Errorf("failed to get expired WG peers: %w", err)
	}

	if len(expiredPeers) == 0 {
		return nil
	}

	m.logger.Info(
		fmt.Sprintf(
			"cleaning up %d expired WireGuard peers",
			len(expiredPeers),
		),
	)

	for _, peer := range expiredPeers {
		// Check for shutdown signal between each peer
		select {
		case <-m.doneChan:
			m.logger.Info("cleanup interrupted by shutdown signal")
			return nil
		default:
		}
		// Log peer identifier safely (handle short pubkeys)
		pubkeyPrefix := peer.Pubkey
		if len(pubkeyPrefix) > 8 {
			pubkeyPrefix = pubkeyPrefix[:8]
		}

		// Skip cleanup if S3 client is not set (S3 is source of truth)
		if m.s3Client == nil {
			m.logger.Warn(
				fmt.Sprintf(
					"skipping cleanup for peer %s: S3 client not configured",
					pubkeyPrefix,
				),
			)
			continue
		}

		// 1. Remove from S3 first (source of truth)
		// If this fails, skip this peer and retry next cycle
		if err := m.s3Client.RemovePeerFromS3(
			peer.AssetName,
			peer.Pubkey,
		); err != nil {
			m.logger.Warn(
				fmt.Sprintf(
					"failed to remove peer %s from S3: %s",
					pubkeyPrefix,
					err,
				),
			)
			continue
		}

		// 2. Remove from DB (cache)
		if err := m.db.DeleteWGPeer(peer.Pubkey); err != nil {
			m.logger.Warn(
				fmt.Sprintf(
					"failed to remove peer %s from DB: %s",
					pubkeyPrefix,
					err,
				),
			)
			// Continue to WG cleanup anyway - S3 is already updated
		}

		// 3. Release IP back to pool for reuse
		if err := m.db.DeallocateIP(
			m.config.Vpn.Region,
			peer.AssignedIP,
		); err != nil {
			m.logger.Warn(
				fmt.Sprintf(
					"failed to deallocate IP %s: %s",
					peer.AssignedIP,
					err,
				),
			)
			// Continue anyway - IP will be reclaimed on next pool wrap-around
		}

		// 4. Remove from WG container (best effort)
		if m.wgClient != nil {
			if err := m.wgClient.RemovePeer(
				peer.Pubkey,
				peer.AssignedIP,
			); err != nil {
				m.logger.Warn(
					fmt.Sprintf(
						"failed to remove peer %s from WG container: %s",
						pubkeyPrefix,
						err,
					),
				)
				// Continue anyway - container will eventually sync
			}
		}

		m.logger.Info(
			fmt.Sprintf("removed expired WG peer %s", pubkeyPrefix),
		)
	}

	return nil
}
