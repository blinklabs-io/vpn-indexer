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
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/vpn-indexer/internal/client"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
	"github.com/blinklabs-io/vpn-indexer/internal/wireguard"
	"github.com/veraison/go-cose"
)

const (
	// WGPubkeyLength is the expected length of a base64-encoded WireGuard public key.
	// WireGuard uses Curve25519 keys which are 32 bytes; base64 encoding of 32 bytes
	// produces exactly 44 characters (including padding).
	WGPubkeyLength = 44

	// WGPubkeyDecodedLength is the expected length of a decoded WireGuard public key.
	// Curve25519 keys are exactly 32 bytes.
	// (Note: also defined below as a const for clarity in validation code)

	// TimestampValidityWindow is the maximum age of a signature timestamp.
	// Set to 60 seconds to minimize replay attack window while allowing for
	// reasonable clock skew between client and server.
	TimestampValidityWindow = 60 * time.Second

	// TimestampFutureSkewWindow is the maximum amount a timestamp can be in
	// the future. This allows for small clock differences between client and
	// server while still preventing replay attacks with future timestamps.
	TimestampFutureSkewWindow = 5 * time.Second

	// DefaultMaxDevices is the default device limit per subscription.
	// Users can register up to this many WireGuard devices per subscription.
	DefaultMaxDevices = 3

	// DefaultDNS is the default DNS server for WireGuard configs.
	// This should point to the VPN server's internal DNS resolver.
	DefaultDNS = "10.8.0.1"

	// RequestTimeout is the maximum time for API request processing.
	// This bounds the total time for all operations in a handler.
	RequestTimeout = 45 * time.Second
)

// WGPubkeyDecodedLength is the byte length of a decoded Curve25519 public key.
const WGPubkeyDecodedLength = 32

// isValidWGPubkey validates a WireGuard public key format
// It checks: valid base64 encoding and decodes to exactly 32 bytes (Curve25519)
func isValidWGPubkey(pubkey string) bool {
	if len(pubkey) != WGPubkeyLength {
		return false
	}
	// Decode base64 and verify it yields exactly 32 bytes
	decoded, err := base64.StdEncoding.DecodeString(pubkey)
	if err != nil {
		return false
	}
	return len(decoded) == WGPubkeyDecodedLength
}

// WGBaseRequest contains the common fields for all WireGuard API requests.
// This is embedded by specific request types that may add additional fields.
// Note: Timestamp validation uses the timestamp embedded in the COSE signature
// payload (clientID + timestamp), not a separate JSON field.
type WGBaseRequest struct {
	ClientID  string `json:"client_id"`
	Signature string `json:"signature"`
	Key       string `json:"key"`
	// Parsed representations (not serialized)
	innerClientID  []byte
	innerSignature cose.UntaggedSign1Message
	innerKey       cose.Key
}

// parseBaseFields parses and validates the hex-encoded fields
func (r *WGBaseRequest) parseBaseFields() error {
	// Client ID
	tmpId, err := hex.DecodeString(r.ClientID)
	if err != nil {
		return errors.New("decode client ID hex")
	}
	r.innerClientID = tmpId

	// Signature
	sigBytes, err := hex.DecodeString(r.Signature)
	if err != nil {
		return errors.New("decode signature hex")
	}
	if err := r.innerSignature.UnmarshalCBOR(sigBytes); err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	// Key
	keyBytes, err := hex.DecodeString(r.Key)
	if err != nil {
		return errors.New("decode key hex")
	}
	if err := r.innerKey.UnmarshalCBOR(keyBytes); err != nil {
		return fmt.Errorf("decode key: %w", err)
	}

	return nil
}

// WireGuard config template
const wgConfigTemplate = `[Interface]
PrivateKey = <REPLACE_WITH_YOUR_PRIVATE_KEY>
Address = %s/32
DNS = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`

// WGRegisterRequest is the request body for WireGuard device registration.
// Embeds WGBaseRequest for common authentication fields.
type WGRegisterRequest struct {
	WGBaseRequest
	WGPubkey string `json:"wg_pubkey"`
}

func (r *WGRegisterRequest) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type alias WGRegisterRequest
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*r = WGRegisterRequest(tmp)
	return r.parseBaseFields()
}

// WGRegisterResponse is the response from WireGuard device registration
type WGRegisterResponse struct {
	Success     bool   `json:"success"`
	AssignedIP  string `json:"assigned_ip"`
	DeviceCount int    `json:"device_count"`
	DeviceLimit int    `json:"device_limit"`
}

// WGProfileRequest is the request body for WireGuard profile generation.
// Embeds WGBaseRequest for common authentication fields.
type WGProfileRequest struct {
	WGBaseRequest
	WGPubkey string `json:"wg_pubkey"`
}

func (r *WGProfileRequest) UnmarshalJSON(data []byte) error {
	type alias WGProfileRequest
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*r = WGProfileRequest(tmp)
	return r.parseBaseFields()
}

// WGDeleteRequest is the request body for WireGuard device deletion.
// Embeds WGBaseRequest for common authentication fields.
type WGDeleteRequest struct {
	WGBaseRequest
	WGPubkey string `json:"wg_pubkey"`
}

func (r *WGDeleteRequest) UnmarshalJSON(data []byte) error {
	type alias WGDeleteRequest
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*r = WGDeleteRequest(tmp)
	return r.parseBaseFields()
}

// WGDeleteResponse is the response from WireGuard device deletion
type WGDeleteResponse struct {
	Success          bool `json:"success"`
	RemainingDevices int  `json:"remaining_devices"`
}

// WGDevicesRequest is the request body for listing WireGuard devices.
// Only uses the base authentication fields, no additional fields needed.
type WGDevicesRequest struct {
	WGBaseRequest
}

func (r *WGDevicesRequest) UnmarshalJSON(data []byte) error {
	type alias WGDevicesRequest
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*r = WGDevicesRequest(tmp)
	return r.parseBaseFields()
}

// WGDevicesResponse is the response containing list of WireGuard devices
type WGDevicesResponse struct {
	Devices []WGDeviceInfo `json:"devices"`
	Limit   int            `json:"limit"`
}

// WGDeviceInfo contains information about a single WireGuard device
type WGDeviceInfo struct {
	Pubkey     string `json:"pubkey"`
	AssignedIP string `json:"assigned_ip"`
	CreatedAt  int64  `json:"created_at"`
}

// ErrorResponse is a JSON error response structure
type ErrorResponse struct {
	Error  string `json:"error"`
	Reason string `json:"reason,omitempty"`
}

// writeErrorResponse writes a properly escaped JSON error response
func writeErrorResponse(w http.ResponseWriter, status int, err, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := ErrorResponse{Error: err, Reason: reason}
	data, _ := json.Marshal(resp)
	_, _ = w.Write(data)
}

// verifyCOSESignature verifies a COSE signature and validates the credential.
// Returns the database client if successful.
func (a *Api) verifyCOSESignature(
	clientID string,
	innerClientID []byte,
	innerSignature *cose.UntaggedSign1Message,
	innerKey *cose.Key,
) (*database.Client, error) {
	// Lookup client in database
	tmpClient, err := a.db.ClientByAssetName(innerClientID)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup client: %w", err)
	}

	// Verify challenge string meets requirements
	// Payload must contain at least clientID + 1 digit for timestamp
	if len(innerSignature.Payload) < len(clientID)+1 {
		return nil, errors.New(
			"challenge payload too short: must contain client ID and timestamp",
		)
	}
	challengeClientId := string(innerSignature.Payload[0:len(clientID)])
	if challengeClientId != clientID {
		return nil, errors.New("challenge string does not match client ID")
	}
	challengeTimestamp := string(
		innerSignature.Payload[len(challengeClientId):],
	)
	tmpTimestamp, err := strconv.ParseInt(challengeTimestamp, 10, 64)
	if err != nil {
		return nil, errors.New(
			"could not extract timestamp from challenge string",
		)
	}
	timestamp := time.Unix(tmpTimestamp, 0)
	age := time.Since(timestamp)
	// Reject timestamps too far in the future (negative age beyond skew window)
	// to prevent replay attacks while allowing small clock differences
	if age < -TimestampFutureSkewWindow {
		return nil, errors.New(
			"challenge string timestamp is too far in the future",
		)
	}
	if age > TimestampValidityWindow {
		return nil, errors.New("challenge string timestamp is too old")
	}

	// Verify challenge signature
	vkey, err := innerKey.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}
	verifier, err := cose.NewVerifier(cose.AlgorithmEdDSA, vkey)
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier: %w", err)
	}
	if err := innerSignature.Verify(nil, verifier); err != nil {
		return nil, errors.New("failed to validate signature")
	}

	// Check that signing key matches known client credential
	ed25519Key, ok := vkey.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("public key is not Ed25519")
	}
	vkeyHash := lcommon.Blake2b224Hash([]byte(ed25519Key))
	if subtle.ConstantTimeCompare(vkeyHash.Bytes(), tmpClient.Credential) != 1 {
		return nil, errors.New(
			"key hash does not match credential for client",
		)
	}

	return &tmpClient, nil
}

// wgRegisterImpl handles POST /api/client/wg-register
//
//	@Summary		WGRegister
//	@Description	Register a new WireGuard device for a client
//	@Accept			json
//	@Produce		json
//	@Param			WGRegisterRequest	body		WGRegisterRequest	true	"Register Request"
//	@Success		200					{object}	WGRegisterResponse	"Registration successful"
//	@Failure		400					{object}	ErrorResponse		"Bad Request (includes generic error for duplicate pubkey to prevent enumeration)"
//	@Failure		401					{object}	ErrorResponse		"Unauthorized"
//	@Failure		403					{object}	ErrorResponse		"Forbidden (device limit reached or subscription expired)"
//	@Failure		405					{object}	string				"Method Not Allowed"
//	@Failure		500					{object}	ErrorResponse		"Server Error"
//	@Router			/api/client/wg-register [post]
func (a *Api) wgRegisterImpl(
	w http.ResponseWriter,
	r *http.Request,
	wgClient *wireguard.Client,
	s3Client *client.Client,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WGRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("failed to decode WG register request", "error", err)
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"malformed request body",
		)
		return
	}

	// Validate WG pubkey is provided
	if req.WGPubkey == "" {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"wg_pubkey is required",
		)
		return
	}
	if !isValidWGPubkey(req.WGPubkey) {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"invalid wg_pubkey format",
		)
		return
	}

	// Verify COSE signature
	tmpClient, err := a.verifyCOSESignature(
		req.ClientID,
		req.innerClientID,
		&req.innerSignature,
		&req.innerKey,
	)
	if err != nil {
		slog.Error("COSE verification failed", "error", err)
		writeErrorResponse(
			w,
			http.StatusUnauthorized,
			"Unauthorized",
			"signature verification failed",
		)
		return
	}

	// Check subscription not expired
	if time.Now().After(tmpClient.Expiration) {
		writeErrorResponse(
			w, http.StatusForbidden, "Forbidden", "subscription has expired",
		)
		return
	}

	maxDevices := a.cfg.Vpn.WGMaxDevices

	// Check if pubkey already registered (fast path)
	existingPeer, err := a.db.GetWGPeerByPubkey(req.WGPubkey)
	if err != nil && !errors.Is(err, database.ErrRecordNotFound) {
		// Actual DB error (not just "not found") - fail the request
		slog.Error("failed to lookup WG peer by pubkey", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}
	if err == nil && existingPeer != nil {
		// Check if pubkey belongs to this client
		if existingPeer.AssetName == nil ||
			string(existingPeer.AssetName) != string(req.innerClientID) {
			// Return generic error to prevent device enumeration attacks
			// Don't reveal that the pubkey exists or belongs to someone else
			slog.Warn(
				"pubkey registration attempt for key owned by another client",
				"client_id", req.ClientID,
			)
			writeErrorResponse(
				w,
				http.StatusBadRequest,
				"Invalid request",
				"unable to register device",
			)
			return
		}
		// Pubkey already registered to this client - return existing info
		deviceCount, countErr := a.db.CountWGPeersByAsset(req.innerClientID)
		if countErr != nil {
			slog.Error("failed to count WG peers", "error", countErr)
			writeErrorResponse(
				w,
				http.StatusInternalServerError,
				"Internal server error",
				"",
			)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		resp := WGRegisterResponse{
			Success:     true,
			AssignedIP:  existingPeer.AssignedIP,
			DeviceCount: int(deviceCount),
			DeviceLimit: maxDevices,
		}
		respBytes, _ := json.Marshal(resp)
		_, _ = w.Write(respBytes)
		return
	}

	// Check device count < limit (only for new registrations)
	deviceCount, err := a.db.CountWGPeersByAsset(req.innerClientID)
	if err != nil {
		slog.Error("failed to count WG peers", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}

	if deviceCount >= int64(maxDevices) {
		writeErrorResponse(
			w,
			http.StatusForbidden,
			"Forbidden",
			"device limit reached",
		)
		return
	}

	// Allocate IP from pool
	assignedIP, err := a.db.AllocateIP(a.cfg.Vpn.Region)
	if err != nil {
		slog.Error("failed to allocate IP", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}

	// Save to S3 first (source of truth)
	// If this fails, nothing is persisted and we must release the allocated IP
	// Use request context with timeout to bound operation time
	if s3Client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
		defer cancel()
		if err := s3Client.SavePeerToS3WithContext(
			ctx,
			req.innerClientID,
			req.WGPubkey,
			assignedIP,
		); err != nil {
			slog.Error("failed to save peer to S3", "error", err)
			// Release the allocated IP back to the pool since S3 save failed
			if deallocErr := a.db.DeallocateIP(
				a.cfg.Vpn.Region,
				assignedIP,
			); deallocErr != nil {
				slog.Error(
					"failed to deallocate IP after S3 failure",
					"ip", assignedIP,
					"error", deallocErr,
				)
			}
			writeErrorResponse(
				w,
				http.StatusInternalServerError,
				"Failed to persist peer",
				"",
			)
			return
		}
	}

	// Save to DB (cache) - if this fails, S3 has the data and
	// the next startup will rebuild the DB from S3.
	// We continue to return success since S3 (source of truth) succeeded.
	if err := a.db.AddWGPeer(
		req.innerClientID,
		req.WGPubkey,
		assignedIP,
	); err != nil {
		slog.Warn(
			"failed to add WG peer to database cache, will sync from S3 on restart",
			"error", err,
			"pubkey", req.WGPubkey[:8]+"...",
		)
		// Continue - S3 is the source of truth and has the peer
	}

	// Call WG container to add peer - best effort, can be retried
	// via SyncPeersToContainer on startup
	if wgClient != nil {
		if _, err := wgClient.AddPeer(req.WGPubkey, assignedIP); err != nil {
			slog.Error("failed to add peer to WG container", "error", err)
			// Continue anyway - peer is registered in S3/DB and will sync on restart
		}
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	resp := WGRegisterResponse{
		Success:     true,
		AssignedIP:  assignedIP,
		DeviceCount: int(deviceCount) + 1,
		DeviceLimit: maxDevices,
	}
	respBytes, _ := json.Marshal(resp)
	_, _ = w.Write(respBytes)
}

// wgProfileImpl handles POST /api/client/wg-profile
//
//	@Summary		WGProfile
//	@Description	Get a WireGuard configuration profile for a registered device
//	@Accept			json
//	@Produce		text/plain,application/json
//	@Param			WGProfileRequest	body		WGProfileRequest	true	"Profile Request"
//	@Success		200					{string}	string				"WireGuard config file"
//	@Failure		400					{object}	ErrorResponse		"Bad Request"
//	@Failure		401					{object}	ErrorResponse		"Unauthorized"
//	@Failure		403					{object}	ErrorResponse		"Forbidden (subscription expired)"
//	@Failure		404					{object}	ErrorResponse		"Not Found"
//	@Failure		405					{object}	string				"Method Not Allowed"
//	@Failure		500					{object}	ErrorResponse		"Server Error"
//	@Router			/api/client/wg-profile [post]
func (a *Api) wgProfileImpl(
	w http.ResponseWriter,
	r *http.Request,
) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WGProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("failed to decode WG profile request", "error", err)
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"malformed request body",
		)
		return
	}

	// Validate WG pubkey is provided
	if req.WGPubkey == "" {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"wg_pubkey is required",
		)
		return
	}
	if !isValidWGPubkey(req.WGPubkey) {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"invalid wg_pubkey format",
		)
		return
	}

	// Verify COSE signature
	tmpClient, err := a.verifyCOSESignature(
		req.ClientID,
		req.innerClientID,
		&req.innerSignature,
		&req.innerKey,
	)
	if err != nil {
		slog.Error("COSE verification failed", "error", err)
		writeErrorResponse(
			w,
			http.StatusUnauthorized,
			"Unauthorized",
			"signature verification failed",
		)
		return
	}

	// Check subscription not expired
	if time.Now().After(tmpClient.Expiration) {
		writeErrorResponse(
			w, http.StatusForbidden, "Forbidden", "subscription has expired",
		)
		return
	}

	// Lookup peer by pubkey - device must be explicitly registered first
	peer, err := a.db.GetWGPeerByPubkey(req.WGPubkey)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			// Device not registered - return 404
			// Users must explicitly register via wg-register endpoint first
			writeErrorResponse(
				w,
				http.StatusNotFound,
				"Not found",
				"device not registered - use wg-register endpoint first",
			)
			return
		}
		slog.Error("failed to lookup WG peer", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}

	// Verify peer belongs to this client
	if peer.AssetName == nil ||
		string(peer.AssetName) != string(req.innerClientID) {
		// Return generic error to prevent device enumeration
		slog.Warn(
			"profile request for key owned by another client",
			"client_id", req.ClientID,
		)
		writeErrorResponse(
			w,
			http.StatusNotFound,
			"Not found",
			"device not registered",
		)
		return
	}

	// Generate WireGuard config
	dns := a.cfg.Vpn.DNS
	if dns == "" {
		dns = DefaultDNS
	}

	serverPubkey := a.cfg.Vpn.WGServerPubkey
	endpoint := a.cfg.Vpn.WGEndpoint

	// Validate WG server config before generating config
	if serverPubkey == "" || endpoint == "" {
		slog.Error(
			"WG server configuration incomplete",
			"serverPubkey_set", serverPubkey != "",
			"endpoint_set", endpoint != "",
		)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Server configuration incomplete",
			"",
		)
		return
	}

	config := fmt.Sprintf(
		wgConfigTemplate,
		peer.AssignedIP,
		dns,
		serverPubkey,
		endpoint,
	)

	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(config))
}

// wgPeerDeleteImpl handles DELETE /api/client/wg-peer
//
//	@Summary		WGPeerDelete
//	@Description	Remove a WireGuard device registration
//	@Accept			json
//	@Produce		json
//	@Param			WGDeleteRequest	body		WGDeleteRequest		true	"Delete Request"
//	@Success		200				{object}	WGDeleteResponse	"Deletion successful"
//	@Failure		400				{object}	ErrorResponse		"Bad Request"
//	@Failure		401				{object}	ErrorResponse		"Unauthorized"
//	@Failure		403				{object}	ErrorResponse		"Forbidden"
//	@Failure		404				{object}	ErrorResponse		"Not Found"
//	@Failure		405				{object}	string				"Method Not Allowed"
//	@Failure		500				{object}	ErrorResponse		"Server Error"
//	@Router			/api/client/wg-peer [delete]
func (a *Api) wgPeerDeleteImpl(
	w http.ResponseWriter,
	r *http.Request,
	wgClient *wireguard.Client,
	s3Client *client.Client,
) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WGDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("failed to decode WG delete request", "error", err)
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"malformed request body",
		)
		return
	}

	// Validate WG pubkey is provided
	if req.WGPubkey == "" {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"wg_pubkey is required",
		)
		return
	}
	if !isValidWGPubkey(req.WGPubkey) {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"invalid wg_pubkey format",
		)
		return
	}

	// Verify COSE signature
	_, err := a.verifyCOSESignature(
		req.ClientID,
		req.innerClientID,
		&req.innerSignature,
		&req.innerKey,
	)
	if err != nil {
		slog.Error("COSE verification failed", "error", err)
		writeErrorResponse(
			w,
			http.StatusUnauthorized,
			"Unauthorized",
			"signature verification failed",
		)
		return
	}

	// Lookup peer by pubkey
	peer, err := a.db.GetWGPeerByPubkey(req.WGPubkey)
	if err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			writeErrorResponse(
				w,
				http.StatusNotFound,
				"Not found",
				"peer not registered",
			)
			return
		}
		slog.Error("failed to lookup WG peer", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}

	// Verify pubkey belongs to this client
	if peer.AssetName == nil ||
		string(peer.AssetName) != string(req.innerClientID) {
		// Return generic error to prevent device enumeration
		slog.Warn(
			"delete request for key owned by another client",
			"client_id", req.ClientID,
		)
		writeErrorResponse(
			w,
			http.StatusNotFound,
			"Not found",
			"device not registered",
		)
		return
	}

	// Remove from S3 first (source of truth)
	// If this fails, nothing is deleted and we return an error
	// Use request context with timeout to bound operation time
	if s3Client != nil {
		ctx, cancel := context.WithTimeout(r.Context(), RequestTimeout)
		defer cancel()
		if err := s3Client.RemovePeerFromS3WithContext(
			ctx,
			req.innerClientID,
			req.WGPubkey,
		); err != nil {
			slog.Error("failed to remove peer from S3", "error", err)
			writeErrorResponse(
				w,
				http.StatusInternalServerError,
				"Failed to remove peer",
				"",
			)
			return
		}
	}

	// Remove from DB (cache) - if this fails, S3 deletion already happened
	// and startup rebuild will sync the state.
	// We continue to return success since S3 (source of truth) succeeded.
	if err := a.db.DeleteWGPeer(req.WGPubkey); err != nil {
		slog.Warn(
			"failed to delete WG peer from database cache, will sync from S3 on restart",
			"error", err,
			"pubkey", req.WGPubkey[:8]+"...",
		)
		// Continue - S3 is the source of truth and the peer is deleted there
	}

	// Release IP back to pool for reuse
	if err := a.db.DeallocateIP(a.cfg.Vpn.Region, peer.AssignedIP); err != nil {
		slog.Warn(
			"failed to deallocate IP",
			"ip", peer.AssignedIP,
			"error", err,
		)
		// Continue anyway - IP will be reclaimed on next pool wrap-around
	}

	// Remove from WG container - best effort, will be cleaned up
	// via SyncPeersToContainer which only adds active peers
	if wgClient != nil {
		if err := wgClient.RemovePeer(peer.Pubkey, peer.AssignedIP); err != nil {
			slog.Error(
				"failed to remove peer from WG container",
				"error",
				err,
			)
			// Continue anyway - container will be out of sync but functional
		}
	}

	// Get remaining device count
	remainingCount, err := a.db.CountWGPeersByAsset(req.innerClientID)
	if err != nil {
		slog.Error("failed to count remaining devices", "error", err)
		remainingCount = 0
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	resp := WGDeleteResponse{
		Success:          true,
		RemainingDevices: int(remainingCount),
	}
	respBytes, _ := json.Marshal(resp)
	_, _ = w.Write(respBytes)
}

// wgDevicesImpl handles POST /api/client/wg-devices
//
//	@Summary		WGDevices
//	@Description	List all WireGuard devices registered for a client
//	@Accept			json
//	@Produce		json
//	@Param			WGDevicesRequest	body		WGDevicesRequest	true	"Devices Request"
//	@Success		200					{object}	WGDevicesResponse	"Device list"
//	@Failure		400					{object}	ErrorResponse		"Bad Request"
//	@Failure		401					{object}	ErrorResponse		"Unauthorized"
//	@Failure		405					{object}	string				"Method Not Allowed"
//	@Failure		500					{object}	ErrorResponse		"Server Error"
//	@Router			/api/client/wg-devices [post]
func (a *Api) wgDevicesImpl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WGDevicesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("failed to decode WG devices request", "error", err)
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"malformed request body",
		)
		return
	}

	// Verify COSE signature
	_, err := a.verifyCOSESignature(
		req.ClientID,
		req.innerClientID,
		&req.innerSignature,
		&req.innerKey,
	)
	if err != nil {
		slog.Error("COSE verification failed", "error", err)
		writeErrorResponse(
			w,
			http.StatusUnauthorized,
			"Unauthorized",
			"signature verification failed",
		)
		return
	}

	// Query DB for peers by asset name
	peers, err := a.db.GetWGPeersByAsset(req.innerClientID)
	if err != nil {
		slog.Error("failed to get WG peers", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}

	// Build device list
	devices := make([]WGDeviceInfo, 0, len(peers))
	for _, peer := range peers {
		devices = append(devices, WGDeviceInfo{
			Pubkey:     peer.Pubkey,
			AssignedIP: peer.AssignedIP,
			CreatedAt:  peer.CreatedAt.Unix(),
		})
	}

	maxDevices := a.cfg.Vpn.WGMaxDevices

	// Return response
	w.Header().Set("Content-Type", "application/json")
	resp := WGDevicesResponse{
		Devices: devices,
		Limit:   maxDevices,
	}
	respBytes, _ := json.Marshal(resp)
	_, _ = w.Write(respBytes)
}
