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
)

// RefDataResponse provides the list of prices and the VPN regions available
type RefDataResponse struct {
	Prices  []RefDataResponsePrice `json:"prices"`
	Regions []string               `json:"regions"`
}

// RefDataResponsePrice provides the price for a given duration
type RefDataResponsePrice struct {
	Duration uint64 `json:"duration"`
	Price    uint64 `json:"price"`
}

// handleRefData godoc
//
//	@Summary		RefData
//	@Description	Fetch prices and regions for signup or renewal
//	@Produce		json
//	@Accept			json
//	@Success		200	{object}	RefDataResponse	"Prices and regions"
//	@Failure		405	{object}	string			"Method Not Allowed"
//	@Failure		500	{object}	string			"Server Error"
//	@Router			/api/refdata [get]
func (a *Api) handleRefData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	refData, err := a.db.ReferenceData()
	if err != nil {
		slog.Error(
			"failed to lookup reference data in database",
			"error",
			err,
		)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		return
	}

	var tmpResp RefDataResponse
	tmpResp.Prices = make([]RefDataResponsePrice, 0, len(refData.Prices))
	for _, price := range refData.Prices {
		tmpResp.Prices = append(
			tmpResp.Prices,
			RefDataResponsePrice{
				Duration: price.Duration,
				Price:    price.Price,
			},
		)
	}
	tmpResp.Regions = make([]string, 0, len(refData.Regions))
	for _, region := range refData.Regions {
		tmpResp.Regions = append(
			tmpResp.Regions,
			region.Name,
		)
	}
	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(tmpResp)
	_, _ = w.Write(resp)
}
