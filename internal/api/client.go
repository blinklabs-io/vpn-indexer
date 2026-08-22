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
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/vpn-indexer/internal/client"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
)

// ClientListRequest provides the payment credential hash to search
type ClientListRequest struct {
	OwnerAddress string `json:"ownerAddress"`
}

// Client provides the unique identifier, expiration time, and VPN region for
// a client
type Client struct {
	Id         string    `json:"id"`
	Expiration time.Time `json:"expiration"`
	Region     string    `json:"region"`
}

// ClientListResponse returns a list of Clients matching the search criteria
type ClientListResponse []Client

// handleClientList godoc
//
//	@Summary		ClientList
//	@Description	Search for clients matching a given manager public key hash
//	@Produce		json
//	@Accept			json
//	@Param			ClientListRequest	body		ClientListRequest	true	"List Request"
//	@Success		200					{object}	ClientListResponse	"List of matching clients"
//	@Failure		400					{object}	string				"Bad Request"
//	@Failure		405					{object}	string				"Method Not Allowed"
//	@Failure		500					{object}	string				"Server Error"
//	@Router			/api/client/list [post]
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

	ownerAddr, err := lcommon.NewAddress(req.OwnerAddress)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(
			[]byte(
				`{"error":"Invalid request","reason":"invalid owner address"}`,
			),
		)
		return
	}
	paymentKeyHash := ownerAddr.PaymentKeyHash().Bytes()
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
	tmpResp := make(ClientListResponse, 0, len(clients))
	for _, client := range clients {
		tmpResp = append(
			tmpResp,
			Client{
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

// ClientProfileRequest names the target subscription; auth is via the session token
type ClientProfileRequest struct {
	Id string `json:"id"`
	// Inner representation (not serialized). Authentication is via a Bearer
	// session token; id only names the target subscription.
	innerId []byte
}

func (r *ClientProfileRequest) UnmarshalJSON(data []byte) error {
	type tmpClientProfileRequest ClientProfileRequest
	var tmpData tmpClientProfileRequest
	if err := json.Unmarshal(data, &tmpData); err != nil {
		return err
	}
	r.Id = tmpData.Id
	id, err := hex.DecodeString(r.Id)
	if err != nil {
		return errors.New("decode client ID hex")
	}
	r.innerId = id
	return nil
}

// handleClientProfile godoc
//
//	@Summary		ClientProfile
//	@Description	Fetch a client VPN profile via a signed S3 link; authenticate with a session Bearer token and provide the subscription id in the body
//	@Accept			json
//	@Param			ClientProfileRequest	body		ClientProfileRequest	true	"Profile Request"
//	@Success		302						{string}	string					"Found"
//	@Failure		400						{object}	string					"Bad Request"
//	@Failure		401						{object}	string					"Unauthorized"
//	@Failure		403						{object}	string					"Forbidden"
//	@Failure		405						{object}	string					"Method Not Allowed"
//	@Failure		500						{object}	string					"Server Error"
//	@Security		BearerAuth
//	@Router			/api/client/profile [post]
func (a *Api) handleClientProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClientProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(
			[]byte(`{"error":"Invalid request","error":"` + err.Error() + `"}`),
		)
		return
	}

	// Authenticate via the Bearer session token and authorise the requested
	// subscription against the token's wallet credential.
	tmpClient, err := a.authenticate(r, req.innerId)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}

	// Reject expired subscriptions, as the WireGuard handlers do.
	if time.Now().After(tmpClient.Expiration) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(
			[]byte(`{"error":"Forbidden","reason":"subscription has expired"}`),
		)
		return
	}

	// Check that profile is available
	client := client.New(a.cfg, a.ca, req.innerId)
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

// ClientAvailableRequest provides the client ID to check for availability
type ClientAvailableRequest struct {
	Id string `json:"id"`
}

// handleClientAvailable godoc
//
//	@Summary		ClientAvailable
//	@Description	Check if a client profile is available
//	@Accept			json
//	@Param			ClientAvailableRequest	body		ClientAvailableRequest	true	"Client Available Request"
//	@Success		200						{string}	string					"OK"
//	@Failure		400						{object}	string					"Bad Request"
//	@Failure		405						{object}	string					"Method Not Allowed"
//	@Failure		500						{object}	string					"Server Error"
//	@Router			/api/client/available [post]
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
		_, _ = w.Write([]byte(`{"msg":"Profile is available"}`))
	} else {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"msg":"Profile is not available"}`))
	}
}
