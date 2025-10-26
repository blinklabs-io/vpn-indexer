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
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/blinklabs-io/vpn-indexer/internal/txbuilder"
)

// TxSignupRequest provides the client address, plan price and duration, and region for the VPN signup
type TxSignupRequest struct {
	PaymentAddress string `json:"paymentAddress"`
	OwnerAddress   string `json:"ownerAddress"`
	Price          int    `json:"price"`
	Duration       int    `json:"duration"`
	Region         string `json:"region"`
}

// TxSignupResponse returns an unsigned transaction for a VPN signup
type TxSignupResponse struct {
	ClientId string `json:"clientId"`
	TxCbor   string `json:"txCbor"`
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

	txCbor, clientId, err := txbuilder.BuildSignupTx(
		txbuilder.SignupDeps{DB: a.db},
		req.PaymentAddress,
		req.OwnerAddress,
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
		var validationErr txbuilder.InputValidationError
		if errors.As(err, &validationErr) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(
				w,
				`{"error":"Invalid request: %s"}`,
				validationErr,
			)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		}
		return
	}

	tmpResp := TxSignupResponse{
		ClientId: hex.EncodeToString(clientId),
		TxCbor:   hex.EncodeToString(txCbor),
	}
	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(tmpResp)
	_, _ = w.Write(resp)
}

// TxRenewRequest provides the existing client ID, plan price and duration, and region for the VPN renewal
type TxRenewRequest struct {
	PaymentAddress string `json:"paymentAddress"`
	OwnerAddress   string `json:"ownerAddress"`
	ClientId       string `json:"clientId"`
	Price          int    `json:"price"`
	Duration       int    `json:"duration"`
}

// TxRenewResponse returns an unsigned transaction for a VPN renewal
type TxRenewResponse struct {
	TxCbor string `json:"txCbor"`
}

// handleTxRenew godoc
//
//	@Summary		TxRenew
//	@Description	Build a transaction for a VPN renewal
//	@Produce		json
//	@Accept			json
//	@Param			TxRenewRequest	body		TxRenewRequest	true	"Renewal Request"
//	@Success		200				{object}	TxRenewResponse	"Built transaction"
//	@Failure		400				{object}	string			"Bad Request"
//	@Failure		405				{object}	string			"Method Not Allowed"
//	@Failure		500				{object}	string			"Server Error"
//	@Router			/api/tx/renew [post]
func (a *Api) handleTxRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TxRenewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request"}`))
		return
	}

	txCbor, err := txbuilder.BuildRenewTransferTx(
		a.db,
		req.PaymentAddress,
		req.OwnerAddress,
		req.ClientId,
		req.Price,
		req.Duration,
	)
	if err != nil {
		slog.Error(
			"failed to build renewal TX",
			"error",
			err,
		)
		var validationErr txbuilder.InputValidationError
		if errors.As(err, &validationErr) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(
				w,
				`{"error":"Invalid request: %s"}`,
				validationErr,
			)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		}
		return
	}

	tmpResp := TxRenewResponse{
		TxCbor: hex.EncodeToString(txCbor),
	}
	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(tmpResp)
	_, _ = w.Write(resp)
}

// TxTransferRequest provides the existing client ID, plan price and duration, and region for the VPN renewal
type TxTransferRequest struct {
	PaymentAddress string `json:"paymentAddress"`
	OwnerAddress   string `json:"ownerAddress"`
	ClientId       string `json:"clientId"`
}

// TxTransferResponse returns an unsigned transaction for a VPN renewal
type TxTransferResponse struct {
	TxCbor string `json:"txCbor"`
}

// handleTxTransfer godoc
//
//	@Summary		TxTransfer
//	@Description	Build a transaction for a VPN transfer
//	@Produce		json
//	@Accept			json
//	@Param			TxTransferRequest	body		TxTransferRequest	true	"Transfer Request"
//	@Success		200					{object}	TxTransferResponse	"Built transaction"
//	@Failure		400					{object}	string				"Bad Request"
//	@Failure		405					{object}	string				"Method Not Allowed"
//	@Failure		500					{object}	string				"Server Error"
//	@Router			/api/tx/transfer [post]
func (a *Api) handleTxTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TxTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid request"}`))
		return
	}

	txCbor, err := txbuilder.BuildRenewTransferTx(
		a.db,
		req.PaymentAddress,
		req.OwnerAddress,
		req.ClientId,
		0,
		0,
	)
	if err != nil {
		slog.Error(
			"failed to build transfer TX",
			"error",
			err,
		)
		var validationErr txbuilder.InputValidationError
		if errors.As(err, &validationErr) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(
				w,
				`{"error":"Invalid request: %s"}`,
				validationErr,
			)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
		}
		return
	}

	tmpResp := TxTransferResponse{
		TxCbor: hex.EncodeToString(txCbor),
	}
	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(tmpResp)
	_, _ = w.Write(resp)
}

// handleTxSubmit godoc
//
//	@Summary		TxSubmit
//	@Description	Submit a signed transaction to the blockchain
//	@Produce		json
//	@Accept			application/cbor
//	@Param			Content-Type	header		string	true	"Content type"	Enums(application/cbor)
//	@Success		200				{object}	string	"Ok"
//	@Failure		400				{object}	string	"Bad Request"
//	@Failure		405				{object}	string	"Method Not Allowed"
//	@Failure		415				{object}	string	"Unsupported Media Type"
//	@Failure		500				{object}	string	"Server Error"
//	@Router			/api/tx/submit [post]
func (a *Api) handleTxSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("Content-Type") != "application/cbor" {
		http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
		return
	}

	// Read raw transaction bytes from the request body and store in a byte array
	txRawBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Server Error", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close() //nolint:errcheck

	txHash, err := txbuilder.SubmitTx(txRawBytes)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s", err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	resp, _ := json.Marshal(txHash)
	_, _ = w.Write(resp)
}
