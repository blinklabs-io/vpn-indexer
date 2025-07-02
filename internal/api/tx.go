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
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/blinklabs-io/vpn-indexer/internal/txbuilder"
)

type TxSignupRequest struct {
	ClientAddress string `json:"clientAddress"`
	Price         uint64 `json:"price"`
	Duration      int64  `json:"duration"`
	Region        string `json:"region"`
}

type TxSignupResponse struct {
	// TODO
}

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

	_, err := txbuilder.BuildSignupTx(
		req.ClientAddress,
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

	// TODO: return txBytes
}
