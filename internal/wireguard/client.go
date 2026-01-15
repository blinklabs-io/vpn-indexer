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
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/database"
	"github.com/blinklabs-io/vpn-indexer/internal/jwt"
)

// Client is an HTTP client for the docker-wireguard peer management API
type Client struct {
	containerURL string
	jwtIssuer    *jwt.Issuer
	httpClient   *http.Client
}

// AddPeerRequest is the request body for adding a peer
type AddPeerRequest struct {
	JWT    string `json:"jwt"`
	Pubkey string `json:"pubkey"`
}

// AddPeerResponse is the response from adding a peer
type AddPeerResponse struct {
	Success      bool   `json:"success"`
	ServerPubkey string `json:"server_pubkey"`
	Endpoint     string `json:"endpoint"`
	AllowedIPs   string `json:"allowed_ips"`
}

// InfoResponse is the response from the info endpoint
type InfoResponse struct {
	ServerPubkey string `json:"server_pubkey"`
	Endpoint     string `json:"endpoint"`
}

// NewClient creates a new WireGuard container client
func NewClient(containerURL string, jwtIssuer *jwt.Issuer) *Client {
	return &Client{
		containerURL: containerURL,
		jwtIssuer:    jwtIssuer,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// buildURL constructs a URL by appending the path to the container URL.
// Handles trailing slashes correctly to avoid double slashes.
func (c *Client) buildURL(path string) (string, error) {
	u, err := url.Parse(c.containerURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse container URL: %w", err)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + path
	return u.String(), nil
}

// AddPeer registers a peer with docker-wireguard (POST /peer)
func (c *Client) AddPeer(pubkey, allowedIP string) (*AddPeerResponse, error) {
	// Generate JWT for authentication
	token, err := c.jwtIssuer.IssuePeerJWT(pubkey, allowedIP)
	if err != nil {
		return nil, fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Create request body
	reqBody := AddPeerRequest{
		JWT:    token,
		Pubkey: pubkey,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL
	peerURL, err := c.buildURL("/peer")
	if err != nil {
		return nil, err
	}

	// Make POST request
	resp, err := c.httpClient.Post(
		peerURL,
		"application/json",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add peer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"add peer request failed with status: %d",
			resp.StatusCode,
		)
	}

	// Parse response
	var result AddPeerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check if the operation was successful
	if !result.Success {
		return nil, fmt.Errorf("add peer failed: server returned success=false")
	}

	return &result, nil
}

// RemovePeer removes a peer from docker-wireguard (DELETE /peer)
// Uses query parameters instead of a JSON body to avoid issues with
// intermediaries that may reject DELETE requests with bodies.
func (c *Client) RemovePeer(pubkey, allowedIP string) error {
	// Generate JWT for authentication
	token, err := c.jwtIssuer.IssuePeerJWT(pubkey, allowedIP)
	if err != nil {
		return fmt.Errorf("failed to generate JWT: %w", err)
	}

	// Build URL with properly encoded query parameters
	peerURL, err := c.buildURL("/peer")
	if err != nil {
		return err
	}
	u, err := url.Parse(peerURL)
	if err != nil {
		return fmt.Errorf("failed to parse peer URL: %w", err)
	}
	q := u.Query()
	q.Set("pubkey", pubkey)
	q.Set("token", token)
	u.RawQuery = q.Encode()

	// Create DELETE request without body
	req, err := http.NewRequest(http.MethodDelete, u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to remove peer: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"remove peer request failed with status: %d",
			resp.StatusCode,
		)
	}

	return nil
}

// GetInfo retrieves server info (GET /info)
func (c *Client) GetInfo() (*InfoResponse, error) {
	infoURL, err := c.buildURL("/info")
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Get(infoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"get info request failed with status: %d",
			resp.StatusCode,
		)
	}

	var result InfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// Health checks container health (GET /health)
func (c *Client) Health() error {
	healthURL, err := c.buildURL("/health")
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Get(healthURL)
	if err != nil {
		return fmt.Errorf("failed to check health: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"health check failed with status: %d",
			resp.StatusCode,
		)
	}

	return nil
}

// SyncPeersToContainer syncs all active peers to the WG container on startup.
// This is called after rebuilding from S3 to ensure the container has all peers.
// Returns an error if more than 50% of sync attempts fail (indicating a systemic issue).
func (c *Client) SyncPeersToContainer(
	db *database.Database,
	region string,
) error {
	// Get all active peers for the region
	peers, err := db.GetActivePeersForRegion(region)
	if err != nil {
		return fmt.Errorf(
			"failed to get active peers for region %s: %w",
			region,
			err,
		)
	}

	if len(peers) == 0 {
		slog.Info("No peers to sync to WG container", "region", region)
		return nil
	}

	slog.Info(
		"Syncing peers to WG container",
		"region", region,
		"count", len(peers),
	)

	successCount := 0
	failCount := 0

	for _, peer := range peers {
		// Add each peer to WG container
		_, err := c.AddPeer(peer.Pubkey, peer.AssignedIP)
		if err != nil {
			// Log but continue - container might already have peer
			// Safely truncate pubkey for logging
			shortPubkey := peer.Pubkey
			if len(shortPubkey) > 8 {
				shortPubkey = shortPubkey[:8] + "..."
			}
			slog.Warn(
				"Failed to sync peer to container",
				"pubkey", shortPubkey,
				"assignedIP", peer.AssignedIP,
				"error", err,
			)
			failCount++
		} else {
			successCount++
		}
	}

	slog.Info(
		"Completed syncing peers to WG container",
		"region", region,
		"success", successCount,
		"failed", failCount,
	)

	// Return error if more than 50% of syncs failed (indicates systemic issue)
	if failCount > 0 && failCount > len(peers)/2 {
		return fmt.Errorf(
			"sync to WG container had high failure rate: %d/%d failed",
			failCount,
			len(peers),
		)
	}

	return nil
}
