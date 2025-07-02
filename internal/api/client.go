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
	"crypto/ed25519"
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
	"github.com/veraison/go-cose"
)

type ClientListRequest struct {
	PaymentKeyHash string `json:"paymentKeyHash"`
	//StakeKeyHash   string `json:"stakeKeyHash"`
}

type ClientListResponse struct {
	Id         string    `json:"id"`
	Expiration time.Time `json:"expiration"`
	Region     string    `json:"region"`
}

func (a *Api) handleClientList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClientListRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request"}`))
		return
	}

	paymentKeyHash, err := hex.DecodeString(req.PaymentKeyHash)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request"}`))
		return
	}
	clients, err := a.db.ClientsByCredential(paymentKeyHash)
	if err != nil {
		slog.Error(
			"failed to lookup client in database",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}
	tmpResp := make([]ClientListResponse, 0, len(clients))
	for _, client := range clients {
		tmpResp = append(
			tmpResp,
			ClientListResponse{
				Id:         hex.EncodeToString(client.AssetName),
				Expiration: client.Expiration,
				Region:     string(client.Region),
			},
		)
	}
	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(tmpResp)
	_, _ = w.Write(resp)
}

type ClientProfileRequest struct {
	Id        []byte
	Signature cose.UntaggedSign1Message
	Key       cose.Key
}

func (r *ClientProfileRequest) UnmarshalJSON(data []byte) error {
	type tmpClientProfileRequest struct {
		Id        string `json:"id"`
		Signature string `json:"signature"`
		Key       string `json:"key"`
	}
	var tmpData tmpClientProfileRequest
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return err
	}
	// Client ID
	tmpId, err := hex.DecodeString(tmpData.Id)
	if err != nil {
		return errors.New("decode client ID hex")
	}
	r.Id = tmpId
	// Signature
	sigBytes, err := hex.DecodeString(tmpData.Signature)
	if err != nil {
		return errors.New("decode signature hex")
	}
	if err := r.Signature.UnmarshalCBOR(sigBytes); err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	// Key
	keyBytes, err := hex.DecodeString(tmpData.Key)
	if err != nil {
		return errors.New("decode key hex")
	}
	if err := r.Key.UnmarshalCBOR(keyBytes); err != nil {
		return fmt.Errorf("decode key: %w", err)
	}
	return nil
}

func (a *Api) handleClientProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClientProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request","error":"` + err.Error() + `"}`))
		return
	}

	// Lookup client in database
	tmpClient, err := a.db.ClientByAssetName(req.Id)
	if err != nil {
		slog.Error(
			"failed to lookup client in database",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}

	// Check that profile is available
	client := client.New(a.cfg, a.ca, req.Id)
	if ok, err := client.ProfileExists(); err != nil {
		slog.Error(
			"failed to check if profile exists",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	} else if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Not found","reason":"client profile doesn't exist"}`))
		return
	}

	// Verify challenge string meets requirements
	challengeClientId := string(req.Signature.Payload[0:len(hex.EncodeToString(req.Id))])
	if challengeClientId != hex.EncodeToString(req.Id) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request","reason":"challenge string does not match client ID"}`))
		return
	}
	challengeTimestamp := string(req.Signature.Payload[len(challengeClientId):])
	tmpTimestamp, err := strconv.ParseInt(challengeTimestamp, 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request","reason":"could not extract timestamp from challenge string"}`))
		return
	}
	timestamp := time.Unix(tmpTimestamp, 0)
	if time.Since(timestamp) > (15 * time.Minute) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request","reason":"challenge string timestamp is too old"}`))
		return
	}

	// Verify challenge signature
	vkey, err := req.Key.PublicKey()
	if err != nil {
		slog.Error(
			"failed to get public key",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}
	verifier, err := cose.NewVerifier(cose.AlgorithmEdDSA, vkey)
	if err != nil {
		slog.Error(
			"failed to create verifier",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}
	if err := req.Signature.Verify(nil, verifier); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request","reason":"failed to validate signature"}`))
		return
	}
	// Check that signing key matches known client credential
	vkeyHash := lcommon.Blake2b224Hash(
		[]byte(
			vkey.(ed25519.PublicKey),
		),
	)
	if string(vkeyHash.Bytes()) != string(tmpClient.Credential) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request","reason":"key hash does not match credential for client"}`))
		return
	}

	// Generate pre-signed S3 URL and redirect
	url, err := client.PresignedUrl()
	if err != nil {
		slog.Error(
			"failed to generate pre-signed URL",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

type ClientAvailableRequest struct {
	Id string `json:"id"`
}

func (a *Api) handleClientAvailable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClientAvailableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request"}`))
		return
	}

	// Lookup client in database
	assetName, err := hex.DecodeString(req.Id)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request"}`))
		return
	}
	if _, err = a.db.ClientByAssetName(assetName); err != nil {
		if errors.Is(err, database.ErrRecordNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slog.Error(
			"failed to lookup client in database",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}

	// Check that profile is available
	client := client.New(a.cfg, a.ca, assetName)
	ok, err := client.ProfileExists()
	if err != nil {
		slog.Error(
			"failed to check if profile exists",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}
	if ok {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}
