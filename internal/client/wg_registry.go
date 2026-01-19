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

package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
)

const peersPrefix = "peers/"

// maxS3Retries is the maximum number of retry attempts for S3 conditional writes
const maxS3Retries = 3

// defaultS3Timeout is the default timeout for S3 operations when no context
// is provided. This prevents indefinite hangs on slow/unresponsive S3.
const defaultS3Timeout = 30 * time.Second

// PeerFile represents a JSON file stored in S3 for a subscription's peers
type PeerFile struct {
	AssetName string     `json:"asset_name"`
	Peers     []PeerInfo `json:"peers"`
	UpdatedAt int64      `json:"updated_at"`
	etag      string     // ETag from S3 for conditional writes (not serialized)
}

// PeerInfo represents a single WireGuard peer registration
type PeerInfo struct {
	Pubkey     string `json:"pubkey"`
	AssignedIP string `json:"assigned_ip"`
	CreatedAt  int64  `json:"created_at"`
}

// SavePeerToS3 adds or updates a peer in the S3 registry.
// Uses ETag-based conditional writes to prevent lost updates from concurrent
// modifications. Uses a default 30s timeout to prevent indefinite hangs.
func (c *Client) SavePeerToS3(
	assetName []byte,
	pubkey, assignedIP string,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultS3Timeout)
	defer cancel()
	return c.SavePeerToS3WithContext(
		ctx,
		assetName,
		pubkey,
		assignedIP,
	)
}

// SavePeerToS3WithContext is like SavePeerToS3 but accepts a context for cancellation.
func (c *Client) SavePeerToS3WithContext(
	ctx context.Context,
	assetName []byte,
	pubkey, assignedIP string,
) error {
	svc, err := c.createS3Client()
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	key := peerFileKey(assetName)

	// Retry loop for handling concurrent modifications
	for attempt := 0; attempt < maxS3Retries; attempt++ {
		// Load existing file or create new
		peerFile, loadErr := c.loadPeerFileFromS3(ctx, svc, key)
		if loadErr != nil {
			return fmt.Errorf("failed to load peer file: %w", loadErr)
		}

		isNewFile := peerFile == nil
		if isNewFile {
			peerFile = &PeerFile{
				AssetName: hex.EncodeToString(assetName),
				Peers:     []PeerInfo{},
			}
		}

		// Add or update peer in slice
		now := time.Now().Unix()
		found := false
		for i, p := range peerFile.Peers {
			if p.Pubkey == pubkey {
				peerFile.Peers[i].AssignedIP = assignedIP
				found = true
				break
			}
		}
		if !found {
			peerFile.Peers = append(peerFile.Peers, PeerInfo{
				Pubkey:     pubkey,
				AssignedIP: assignedIP,
				CreatedAt:  now,
			})
		}
		peerFile.UpdatedAt = now

		// Upload JSON to S3 with conditional write
		data, marshalErr := json.Marshal(peerFile)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal peer file: %w", marshalErr)
		}

		putInput := &s3.PutObjectInput{
			Bucket:      aws.String(c.config.S3.ClientBucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String("application/json"),
		}

		// Use conditional write based on whether file existed
		if isNewFile {
			// For new files, only succeed if file doesn't exist
			putInput.IfNoneMatch = aws.String("*")
		} else {
			// For existing files, only succeed if ETag matches
			putInput.IfMatch = aws.String(peerFile.etag)
		}

		_, putErr := svc.PutObject(ctx, putInput)
		if putErr != nil {
			// Check if this is a precondition failure (concurrent modification)
			// S3 returns HTTP 412 PreconditionFailed or 409 ConditionalRequestConflict
			var apiErr smithy.APIError
			if errors.As(putErr, &apiErr) {
				code := apiErr.ErrorCode()
				if code == "PreconditionFailed" ||
					code == "ConditionalRequestConflict" {
					slog.Debug(
						"S3 conditional write failed, retrying",
						"attempt", attempt+1,
						"key", key,
						"code", code,
					)
					continue // Retry with fresh data
				}
			}
			// For other errors, return immediately
			return fmt.Errorf("failed to upload peer file to S3: %w", putErr)
		}

		// Success
		return nil
	}

	return fmt.Errorf(
		"failed to save peer file after %d retries due to concurrent modifications",
		maxS3Retries,
	)
}

// RemovePeerFromS3 removes a peer from the S3 registry.
// Uses ETag-based conditional writes to prevent lost updates from concurrent
// modifications. Uses a default 30s timeout to prevent indefinite hangs.
func (c *Client) RemovePeerFromS3(assetName []byte, pubkey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultS3Timeout)
	defer cancel()
	return c.RemovePeerFromS3WithContext(ctx, assetName, pubkey)
}

// RemovePeerFromS3WithContext is like RemovePeerFromS3 but accepts a context
// for cancellation.
func (c *Client) RemovePeerFromS3WithContext(
	ctx context.Context,
	assetName []byte,
	pubkey string,
) error {
	svc, err := c.createS3Client()
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	key := peerFileKey(assetName)

	// Retry loop for handling concurrent modifications
	for attempt := 0; attempt < maxS3Retries; attempt++ {
		// Load existing file
		peerFile, loadErr := c.loadPeerFileFromS3(ctx, svc, key)
		if loadErr != nil {
			return fmt.Errorf("failed to load peer file: %w", loadErr)
		}

		if peerFile == nil {
			// No file exists, nothing to remove
			return nil
		}

		// Remove peer by pubkey
		newPeers := make([]PeerInfo, 0, len(peerFile.Peers))
		for _, p := range peerFile.Peers {
			if p.Pubkey != pubkey {
				newPeers = append(newPeers, p)
			}
		}

		// If no peers left, save empty file rather than deleting.
		// This avoids a TOCTOU race condition: S3 DeleteObject is not conditional,
		// so a concurrent SavePeerToS3 between our check and delete would lose data.
		// Empty peer files are ~50 bytes, so storage cost is negligible.
		if len(newPeers) == 0 {
			peerFile.Peers = newPeers
			peerFile.UpdatedAt = time.Now().Unix()

			data, marshalErr := json.Marshal(peerFile)
			if marshalErr != nil {
				return fmt.Errorf("failed to marshal empty peer file: %w", marshalErr)
			}

			putInput := &s3.PutObjectInput{
				Bucket:      aws.String(c.config.S3.ClientBucket),
				Key:         aws.String(key),
				Body:        bytes.NewReader(data),
				ContentType: aws.String("application/json"),
				IfMatch:     aws.String(peerFile.etag), // Conditional write
			}

			_, putErr := svc.PutObject(ctx, putInput)
			if putErr != nil {
				var apiErr smithy.APIError
				if errors.As(putErr, &apiErr) {
					code := apiErr.ErrorCode()
					if code == "PreconditionFailed" ||
						code == "ConditionalRequestConflict" {
						slog.Debug(
							"S3 conditional write failed for empty file, retrying",
							"attempt", attempt+1,
							"key", key,
							"code", code,
						)
						continue // Retry with fresh data
					}
				}
				return fmt.Errorf(
					"failed to save empty peer file to S3: %w",
					putErr,
				)
			}
			return nil
		}

		// Otherwise update the file with conditional write
		peerFile.Peers = newPeers
		peerFile.UpdatedAt = time.Now().Unix()

		data, marshalErr := json.Marshal(peerFile)
		if marshalErr != nil {
			return fmt.Errorf("failed to marshal peer file: %w", marshalErr)
		}

		putInput := &s3.PutObjectInput{
			Bucket:      aws.String(c.config.S3.ClientBucket),
			Key:         aws.String(key),
			Body:        bytes.NewReader(data),
			ContentType: aws.String("application/json"),
			IfMatch:     aws.String(peerFile.etag), // Conditional write
		}

		_, putErr := svc.PutObject(ctx, putInput)
		if putErr != nil {
			// Check if this is a precondition failure (concurrent modification)
			// S3 returns HTTP 412 PreconditionFailed or 409 ConditionalRequestConflict
			var apiErr smithy.APIError
			if errors.As(putErr, &apiErr) {
				code := apiErr.ErrorCode()
				if code == "PreconditionFailed" ||
					code == "ConditionalRequestConflict" {
					slog.Debug(
						"S3 conditional write failed, retrying",
						"attempt", attempt+1,
						"key", key,
						"code", code,
					)
					continue // Retry with fresh data
				}
			}
			// For other errors, return immediately
			return fmt.Errorf("failed to update peer file in S3: %w", putErr)
		}

		// Success
		return nil
	}

	return fmt.Errorf(
		"failed to remove peer after %d retries due to concurrent modifications",
		maxS3Retries,
	)
}

// LoadPeersFromS3 loads and parses a peer file from S3.
// Returns nil if the file is not found (not an error).
// Uses a default 30s timeout to prevent indefinite hangs.
func (c *Client) LoadPeersFromS3(assetName []byte) (*PeerFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultS3Timeout)
	defer cancel()
	svc, err := c.createS3Client()
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	key := peerFileKey(assetName)
	return c.loadPeerFileFromS3(ctx, svc, key)
}

// ListAllPeerFiles lists all keys with prefix "peers/"
// Uses a 2 minute timeout as listing can take longer with many files.
func (c *Client) ListAllPeerFiles() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	svc, err := c.createS3Client()
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	var keys []string
	paginator := s3.NewListObjectsV2Paginator(svc, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.config.S3.ClientBucket),
		Prefix: aws.String(peersPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list peer files from S3: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}

	return keys, nil
}

// loadPeerFileFromS3 loads a peer file from S3 by key.
// Returns nil if the file is not found.
// The returned PeerFile includes the ETag for conditional writes.
func (c *Client) loadPeerFileFromS3(
	ctx context.Context,
	svc *s3.Client,
	key string,
) (*PeerFile, error) {
	result, err := svc.GetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(c.config.S3.ClientBucket),
			Key:    aws.String(key),
		},
	)
	if err != nil {
		var nfErr *s3types.NoSuchKey
		if errors.As(err, &nfErr) {
			return nil, nil
		}
		// Also check for NotFound error type
		var notFoundErr *s3types.NotFound
		if errors.As(err, &notFoundErr) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = result.Body.Close() }()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read peer file body: %w", err)
	}

	var peerFile PeerFile
	if err := json.Unmarshal(data, &peerFile); err != nil {
		return nil, fmt.Errorf("failed to unmarshal peer file: %w", err)
	}

	// Store the ETag for conditional writes
	if result.ETag != nil {
		peerFile.etag = *result.ETag
	}

	return &peerFile, nil
}

// peerFileKey generates the S3 key for a peer file
func peerFileKey(assetName []byte) string {
	return fmt.Sprintf("%s%s.json", peersPrefix, hex.EncodeToString(assetName))
}

// RebuildWGPeersFromS3 loads all peer files from S3 and populates the database.
// This is called on startup when the database is empty (ephemeral indexer support).
func (c *Client) RebuildWGPeersFromS3(
	db *database.Database,
	region string,
) error {
	slog.Info("Rebuilding WG peers from S3...")

	// 1. List all peer files using ListAllPeerFiles()
	keys, err := c.ListAllPeerFiles()
	if err != nil {
		return fmt.Errorf("failed to list peer files from S3: %w", err)
	}

	slog.Info("Found peer files in S3", "count", len(keys))

	// 2. For each file, load using LoadPeersFromS3 (extract asset name from key)
	loadedCount := 0
	for _, key := range keys {
		// Extract asset name hex from key (format: peers/{hex_asset_name}.json)
		assetNameHex := extractAssetNameFromKey(key)
		if assetNameHex == "" {
			slog.Warn(
				"Failed to extract asset name from key, skipping",
				"key", key,
			)
			continue
		}

		assetName, err := hex.DecodeString(assetNameHex)
		if err != nil {
			slog.Warn(
				"Failed to decode asset name hex, skipping",
				"key", key,
				"error", err,
			)
			continue
		}

		// Load the peer file
		peerFile, err := c.LoadPeersFromS3(assetName)
		if err != nil {
			slog.Warn(
				"Failed to load peer file from S3, skipping",
				"key", key,
				"error", err,
			)
			continue
		}

		if peerFile == nil {
			slog.Warn("Peer file is nil, skipping", "key", key)
			continue
		}

		// 3. For each peer in the file, call db.AddWGPeer()
		for _, peer := range peerFile.Peers {
			if err := db.AddWGPeer(
				assetName,
				peer.Pubkey,
				peer.AssignedIP,
			); err != nil {
				// Truncate pubkey safely for logging
				pubkeyPrefix := peer.Pubkey
				if len(pubkeyPrefix) > 8 {
					pubkeyPrefix = pubkeyPrefix[:8] + "..."
				}
				slog.Warn(
					"Failed to add WG peer to database",
					"pubkey", pubkeyPrefix,
					"error", err,
				)
				continue
			}
			loadedCount++
		}
	}

	slog.Info("Loaded WG peers from S3", "count", loadedCount)

	// 4. After all peers loaded, call db.RebuildIPPool(region)
	if err := db.RebuildIPPool(region); err != nil {
		return fmt.Errorf("failed to rebuild IP pool: %w", err)
	}

	slog.Info("Rebuilt IP pool for region", "region", region)

	return nil
}

// extractAssetNameFromKey extracts the hex asset name from an S3 key.
// Key format: peers/{hex_asset_name}.json
func extractAssetNameFromKey(key string) string {
	// Remove "peers/" prefix
	if !strings.HasPrefix(key, peersPrefix) {
		return ""
	}
	name := strings.TrimPrefix(key, peersPrefix)

	// Remove ".json" suffix
	if !strings.HasSuffix(name, ".json") {
		return ""
	}
	name = strings.TrimSuffix(name, ".json")

	return name
}
