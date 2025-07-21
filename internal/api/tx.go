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
	"log/slog"
	"net/http"

	"github.com/blinklabs-io/vpn-indexer/internal/txbuilder"
)

// TxSignupRequest provides the client address, plan price and duration, and region for the VPN signup
type TxSignupRequest struct {
	ClientAddress string `json:"clientAddress"`
	Price         int    `json:"price"`
	Duration      int    `json:"duration"`
	Region        string `json:"region"`
}

// TxSignupResponse returns an unsigned transaction for a VPN signup
type TxSignupResponse struct {
	TxCbor string `json:"txCbor"`
}

// handleTxSignup godoc
//
//	@Summary		TxSignup
//	@Description	Build a transaction for a VPN signup
//	@Produce		json
//	@Accept			json
//	@Param			TxSignupRequest	body		TxSignupRequest		true	"Signup Request"
//	@Success		200				{object}	TxSignupResponse	"Built transaction"
//	@Failure		400				{object}	string				"Bad Request"
//	@Failure		405				{object}	string				"Method Not Allowed"
//	@Failure		500				{object}	string				"Server Error"
//	@Router			/api/tx/signup [post]
func (a *Api) handleTxSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TxSignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request"}`))
		return
	}

	txCbor, err := txbuilder.BuildSignupTx(
		a.db,
		req.ClientAddress,
		req.Price,
		req.Duration,
		req.Region,
	)
	if err != nil {
		slog.Error(
			"failed to build signup TX",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}

	tmpResp := TxSignupResponse{
		TxCbor: hex.EncodeToString(txCbor),
	}
	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(tmpResp)
	_, _ = w.Write(resp)
}
