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
	"time"
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
