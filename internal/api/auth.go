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
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
	"github.com/veraison/go-cose"
)

// SessionRequest is the request body for POST /api/auth/session. It carries a
// wallet-signed challenge; the issued token is scoped to the wallet credential
// derived from the signing key.
type SessionRequest struct {
	Signature string `json:"signature" binding:"required"`
	Key       string `json:"key"       binding:"required"`

	// Parsed representations (not serialized).
	innerSignature cose.UntaggedSign1Message
	innerKey       cose.Key
}

func (r *SessionRequest) UnmarshalJSON(data []byte) error {
	type alias SessionRequest
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*r = SessionRequest(tmp)

	// Decode the signature and key from the challenge.
	var err error
	r.innerSignature, r.innerKey, err = parseCOSESignature(r.Signature, r.Key)
	return err
}

// parseCOSESignature decodes the hex-encoded COSE signature and key from a
// session challenge request. Either may be empty, in which case the zero value
// is returned for it.
func parseCOSESignature(
	signature, key string,
) (cose.UntaggedSign1Message, cose.Key, error) {
	var (
		innerSignature cose.UntaggedSign1Message
		innerKey       cose.Key
	)
	if signature != "" {
		sigBytes, err := hex.DecodeString(signature)
		if err != nil {
			return innerSignature, innerKey, fmt.Errorf(
				"decode signature hex: %w", err,
			)
		}
		if err := innerSignature.UnmarshalCBOR(sigBytes); err != nil {
			return innerSignature, innerKey, fmt.Errorf(
				"decode signature: %w", err,
			)
		}
	}
	if key != "" {
		keyBytes, err := hex.DecodeString(key)
		if err != nil {
			return innerSignature, innerKey, fmt.Errorf(
				"decode key hex: %w", err,
			)
		}
		if err := innerKey.UnmarshalCBOR(keyBytes); err != nil {
			return innerSignature, innerKey, fmt.Errorf("decode key: %w", err)
		}
	}
	return innerSignature, innerKey, nil
}

// sessionChallengePrefix identifies the session challenge payload, separating
// it from any other signed material a wallet might produce.
const sessionChallengePrefix = "vpn-session:"

// validateChallengeTimestamp checks that a unix-timestamp string falls within
// the accepted freshness window.
//
// Because the service stores no nonces, this window is the sole replay
// defence: a captured signed challenge is replayable only until its timestamp
// ages out. The small future-skew allowance tolerates clock drift.
func validateChallengeTimestamp(ts string) error {
	tmpTimestamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return errors.New(
			"could not extract timestamp from challenge string",
		)
	}
	age := time.Since(time.Unix(tmpTimestamp, 0))
	// Reject timestamps too far in the future (negative age beyond the skew
	// window) to prevent replay attacks while allowing small clock differences.
	if age < -TimestampFutureSkewWindow {
		return errors.New(
			"challenge string timestamp is too far in the future",
		)
	}
	if age > TimestampValidityWindow {
		return errors.New("challenge string timestamp is too old")
	}
	return nil
}

// validateSessionChallenge verifies that a COSE payload of the form
// "vpn-session:<unixTimestamp>" carries a fresh timestamp.
func validateSessionChallenge(payload []byte) error {
	s := string(payload)
	if !strings.HasPrefix(s, sessionChallengePrefix) {
		return errors.New("invalid session challenge")
	}
	return validateChallengeTimestamp(s[len(sessionChallengePrefix):])
}

// verifySessionChallenge verifies a wallet-signed session challenge and returns
// the credential (Blake2b-224 hash of the signing key, i.e. the payment key
// hash) it resolves to. This credential is the identity a session token is
// bound to; it covers every subscription owned by that credential.
func (a *Api) verifySessionChallenge(
	innerSignature *cose.UntaggedSign1Message,
	innerKey *cose.Key,
) ([]byte, error) {
	if err := validateSessionChallenge(innerSignature.Payload); err != nil {
		return nil, err
	}

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

	ed25519Key, ok := vkey.(ed25519.PublicKey)
	if !ok {
		return nil, errors.New("public key is not Ed25519")
	}
	return lcommon.Blake2b224Hash([]byte(ed25519Key)).Bytes(), nil
}

// SessionResponse is the response from POST /api/auth/session.
type SessionResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// handleAuthSession handles POST /api/auth/session
//
//	@Summary		AuthSession
//	@Description	Exchange a wallet-signed challenge for a short-lived session token covering all of the wallet's subscriptions
//	@Accept			json
//	@Produce		json
//	@Param			SessionRequest	body		SessionRequest	true	"Session Request"
//	@Success		200				{object}	SessionResponse	"Session token"
//	@Failure		400				{object}	ErrorResponse	"Bad Request"
//	@Failure		401				{object}	ErrorResponse	"Unauthorized"
//	@Failure		403				{object}	ErrorResponse	"Forbidden (no subscriptions for wallet)"
//	@Failure		405				{object}	string			"Method Not Allowed"
//	@Failure		500				{object}	ErrorResponse	"Server Error"
//	@Router			/api/auth/session [post]
func (a *Api) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Debug("failed to decode session request", "error", err)
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"malformed request body",
		)
		return
	}

	// A session is established with a fresh wallet signature; the token is
	// bound to the wallet credential derived from the signing key.
	if req.Signature == "" || req.Key == "" {
		writeErrorResponse(
			w,
			http.StatusBadRequest,
			"Invalid request",
			"signature and key are required",
		)
		return
	}

	// Verify the signed challenge and derive the wallet credential.
	credential, err := a.verifySessionChallenge(
		&req.innerSignature,
		&req.innerKey,
	)
	if err != nil {
		slog.Error("session challenge verification failed", "error", err)
		writeErrorResponse(
			w,
			http.StatusUnauthorized,
			"Unauthorized",
			"signature verification failed",
		)
		return
	}

	// The credential must own at least one subscription: this rejects keys
	// unrelated to any subscription and confirms the wallet is known.
	clients, err := a.db.ClientsByCredential(credential)
	if err != nil {
		slog.Error("failed to lookup clients by credential", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}
	if len(clients) == 0 {
		writeErrorResponse(
			w,
			http.StatusForbidden,
			"Forbidden",
			"no subscriptions for this wallet",
		)
		return
	}

	token, expiresAt, err := a.jwtIssuer.IssueSessionJWT(
		hex.EncodeToString(credential),
	)
	if err != nil {
		slog.Error("failed to issue session token", "error", err)
		writeErrorResponse(
			w,
			http.StatusInternalServerError,
			"Internal server error",
			"",
		)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// The response carries a bearer credential; never let it be cached.
	w.Header().Set("Cache-Control", "no-store")
	resp := SessionResponse{
		Token:     token,
		ExpiresAt: expiresAt.Unix(),
	}
	respBytes, _ := json.Marshal(resp)
	_, _ = w.Write(respBytes)
}

// bearerToken extracts a Bearer token from the Authorization header.
// Returns an empty string when no Bearer token is present.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// sessionCredential resolves the wallet credential carried by a Bearer session
// token.
//
// present is false when no Bearer token is sent. When a token is present but
// invalid the bool is true and err is non-nil. The returned credential
// authorises any subscription whose Client.Credential matches it.
func (a *Api) sessionCredential(
	r *http.Request,
) (credential []byte, present bool, err error) {
	token := bearerToken(r)
	if token == "" {
		return nil, false, nil
	}
	credentialHex, err := a.jwtIssuer.VerifySessionJWT(token)
	if err != nil {
		return nil, true, fmt.Errorf("invalid session token: %w", err)
	}
	credential, err = hex.DecodeString(credentialHex)
	if err != nil {
		return nil, true, errors.New("invalid credential in session token")
	}
	return credential, true, nil
}

// authorizeClient confirms the named subscription belongs to the wallet
// credential and returns it. Identity (the credential) comes from the token;
// the body only names which subscription to act on, so the ownership check is
// what prevents a token from acting on another wallet's subscription.
func (a *Api) authorizeClient(
	credential []byte,
	innerClientID []byte,
) (*database.Client, error) {
	if len(innerClientID) == 0 {
		return nil, errors.New("client_id is required")
	}
	tmpClient, err := a.db.ClientByAssetName(innerClientID)
	if err != nil {
		// A missing record is an auth/ownership failure; any other error is an
		// infrastructure problem that must surface as 500, not 401.
		if errors.Is(err, database.ErrRecordNotFound) {
			return nil, errors.New("session token does not own this subscription")
		}
		return nil, fmt.Errorf("%w: lookup client: %w", errAuthInternal, err)
	}
	if !bytes.Equal(tmpClient.Credential, credential) {
		return nil, errors.New("session token does not own this subscription")
	}
	return &tmpClient, nil
}

// errAuthInternal wraps an internal failure (e.g. a database error) that occurs
// while authenticating a request, so handlers can return 500 rather than
// masking infrastructure problems as 401 Unauthorized.
var errAuthInternal = errors.New("internal authentication error")

// writeAuthError maps an authenticate() error to an HTTP response: internal
// failures return 500, all other (invalid token / ownership) failures return
// 401.
func (a *Api) writeAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, errAuthInternal) {
		slog.Error("authentication error", "error", err)
		writeErrorResponse(
			w, http.StatusInternalServerError, "Internal server error", "",
		)
		return
	}
	slog.Warn("authentication failed", "error", err)
	writeErrorResponse(
		w, http.StatusUnauthorized, "Unauthorized", "authentication failed",
	)
}

// authenticate resolves and authorizes the client for a protected request. It
// requires a valid Bearer session token and confirms the named subscription
// (innerClientID) belongs to the token's wallet credential.
func (a *Api) authenticate(
	r *http.Request,
	innerClientID []byte,
) (*database.Client, error) {
	credential, present, err := a.sessionCredential(r)
	if !present {
		return nil, errors.New("session token required")
	}
	if err != nil {
		return nil, err
	}
	return a.authorizeClient(credential, innerClientID)
}
