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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsValidWGPubkey(t *testing.T) {
	tests := []struct {
		name    string
		pubkey  string
		isValid bool
	}{
		{
			// Real WireGuard pubkeys are base64 encoded 32-byte Curve25519 keys
			// 32 bytes = 256 bits, base64 encodes to 44 chars with padding
			name:    "valid pubkey format",
			pubkey:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			isValid: true,
		},
		{
			name:    "valid pubkey all Zs",
			pubkey:  "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZY=",
			isValid: true,
		},
		{
			name:    "too short",
			pubkey:  "ABCDEF==",
			isValid: false,
		},
		{
			name:    "too long",
			pubkey:  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrst==",
			isValid: false,
		},
		{
			name:    "invalid base64 characters",
			pubkey:  "ABCDEFGHIJKLMNOPQRSTUVWXYZ!@#$%^&*()_+==",
			isValid: false,
		},
		{
			name:    "empty string",
			pubkey:  "",
			isValid: false,
		},
		{
			name:    "correct length but decodes to wrong size",
			pubkey:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			isValid: false, // No padding, decodes to 33 bytes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidWGPubkey(tt.pubkey)
			if result != tt.isValid {
				t.Errorf(
					"isValidWGPubkey(%q) = %v, want %v",
					tt.pubkey,
					result,
					tt.isValid,
				)
			}
		})
	}
}

func TestWGRegisterRequestUnmarshal(t *testing.T) {
	// Test that invalid fields cause appropriate errors
	tests := []struct {
		name        string
		json        string
		shouldError bool
	}{
		{
			name:        "empty object",
			json:        `{}`,
			shouldError: true, // hex decode fails on empty string
		},
		{
			name:        "invalid client_id hex",
			json:        `{"client_id": "not-hex", "wg_pubkey": "test", "timestamp": 123, "signature": "abc", "key": "def"}`,
			shouldError: true,
		},
		{
			name:        "invalid signature hex",
			json:        `{"client_id": "0123456789abcdef", "wg_pubkey": "testkey", "timestamp": 123, "signature": "not-hex", "key": "abcd"}`,
			shouldError: true,
		},
		{
			name:        "valid hex but invalid CBOR",
			json:        `{"client_id": "0123456789abcdef", "wg_pubkey": "testkey", "timestamp": 123, "signature": "abcd", "key": "1234"}`,
			shouldError: true, // CBOR unmarshalling will fail
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req WGRegisterRequest
			err := json.Unmarshal([]byte(tt.json), &req)
			if tt.shouldError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestWriteErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		errMsg     string
		reason     string
		wantStatus int
		wantJSON   bool
	}{
		{
			name:       "bad request with reason",
			status:     http.StatusBadRequest,
			errMsg:     "Invalid request",
			reason:     "missing field",
			wantStatus: http.StatusBadRequest,
			wantJSON:   true,
		},
		{
			name:       "unauthorized without reason",
			status:     http.StatusUnauthorized,
			errMsg:     "Unauthorized",
			reason:     "",
			wantStatus: http.StatusUnauthorized,
			wantJSON:   true,
		},
		{
			name:       "internal error",
			status:     http.StatusInternalServerError,
			errMsg:     "Internal server error",
			reason:     "",
			wantStatus: http.StatusInternalServerError,
			wantJSON:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeErrorResponse(w, tt.status, tt.errMsg, tt.reason)

			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			if tt.wantJSON {
				contentType := w.Header().Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf(
						"Content-Type = %q, want %q",
						contentType,
						"application/json",
					)
				}

				var resp ErrorResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Errorf("failed to parse response JSON: %v", err)
				}
				if resp.Error != tt.errMsg {
					t.Errorf("error = %q, want %q", resp.Error, tt.errMsg)
				}
				if resp.Reason != tt.reason {
					t.Errorf("reason = %q, want %q", resp.Reason, tt.reason)
				}
			}
		})
	}
}

func TestWGConfigTemplate(t *testing.T) {
	// Verify the template contains expected placeholders
	if !strings.Contains(wgConfigTemplate, "[Interface]") {
		t.Error("template missing [Interface] section")
	}
	if !strings.Contains(wgConfigTemplate, "[Peer]") {
		t.Error("template missing [Peer] section")
	}
	if !strings.Contains(wgConfigTemplate, "PrivateKey") {
		t.Error("template missing PrivateKey field")
	}
	if !strings.Contains(wgConfigTemplate, "PublicKey") {
		t.Error("template missing PublicKey field")
	}
	if !strings.Contains(wgConfigTemplate, "Endpoint") {
		t.Error("template missing Endpoint field")
	}
	if !strings.Contains(wgConfigTemplate, "AllowedIPs") {
		t.Error("template missing AllowedIPs field")
	}
}

func TestWGBaseRequestParseFields(t *testing.T) {
	tests := []struct {
		name        string
		req         WGBaseRequest
		shouldError bool
	}{
		{
			name: "invalid client_id hex",
			req: WGBaseRequest{
				ClientID:  "not-valid-hex",
				Signature: "abcd",
				Key:       "1234",
			},
			shouldError: true,
		},
		{
			name: "invalid signature hex",
			req: WGBaseRequest{
				ClientID:  "0123456789abcdef",
				Signature: "not-valid-hex",
				Key:       "1234",
			},
			shouldError: true,
		},
		{
			name: "invalid key hex",
			req: WGBaseRequest{
				ClientID:  "0123456789abcdef",
				Signature: "abcd",
				Key:       "not-valid-hex",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.parseBaseFields()
			if tt.shouldError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConstants(t *testing.T) {
	// Verify constants have sensible values
	if WGPubkeyLength != 44 {
		t.Errorf("WGPubkeyLength = %d, want 44", WGPubkeyLength)
	}
	if WGPubkeyDecodedLength != 32 {
		t.Errorf("WGPubkeyDecodedLength = %d, want 32", WGPubkeyDecodedLength)
	}
	if DefaultMaxDevices < 1 {
		t.Errorf(
			"DefaultMaxDevices = %d, want at least 1",
			DefaultMaxDevices,
		)
	}
	if TimestampValidityWindow.Seconds() < 30 {
		t.Errorf(
			"TimestampValidityWindow = %v, want at least 30s",
			TimestampValidityWindow,
		)
	}
	if TimestampValidityWindow.Minutes() > 5 {
		t.Errorf(
			"TimestampValidityWindow = %v, want at most 5 minutes",
			TimestampValidityWindow,
		)
	}
}
