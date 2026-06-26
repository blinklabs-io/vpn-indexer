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

package jwt

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Issuer handles Ed25519 JWT signing for WireGuard peer authentication
// and browser session tokens.
type Issuer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
}

// NewIssuer loads an Ed25519 private key from a PEM file
func NewIssuer(keyFile string) (*Issuer, error) {
	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, errors.New("failed to decode PEM data")
	}

	// Parse the private key
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	ed25519Key, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an Ed25519 private key")
	}

	// ed25519.PrivateKey.Public() always returns an ed25519.PublicKey.
	publicKey := ed25519Key.Public().(ed25519.PublicKey)

	return &Issuer{
		privateKey: ed25519Key,
		publicKey:  publicKey,
	}, nil
}

// PeerJWTLifetime is the validity period for peer management JWTs.
// Set to 5 minutes to allow for network latency, retries, and clock skew
// between the indexer and WireGuard container.
const PeerJWTLifetime = 5 * time.Minute

// IssuePeerJWT creates a short-lived JWT for WG peer operations.
// Claims: sub="wg_peer", pubkey, allowed_ip, iat, exp
func (i *Issuer) IssuePeerJWT(pubkey, allowedIP string) (string, error) {
	now := time.Now()

	claims := jwt.MapClaims{
		"sub":        "wg_peer",
		"pubkey":     pubkey,
		"allowed_ip": allowedIP,
		"iat":        now.Unix(),
		"exp":        now.Add(PeerJWTLifetime).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	signedToken, err := token.SignedString(i.privateKey)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

// SessionJWTLifetime is the validity period for browser session tokens.
// Kept short so the lack of explicit revocation is acceptable; subscription
// expiry is still re-checked against the database on every operation.
const SessionJWTLifetime = 30 * time.Minute

// sessionAudience identifies session tokens, distinguishing them from peer
// tokens so the two can never be used interchangeably.
const sessionAudience = "session"

// IssueSessionJWT creates a short-lived session token bound to a subject
// (the wallet credential). It returns the signed token and the token's
// authoritative expiry time, so callers can report expiry without re-deriving
// it from a second clock read (which could differ from the exp claim by up to
// a second).
// Claims: sub=<subject>, aud="session", iat, exp
func (i *Issuer) IssueSessionJWT(subject string) (string, time.Time, error) {
	if subject == "" {
		return "", time.Time{}, errors.New("subject is required")
	}
	now := time.Now()
	expiresAt := now.Add(SessionJWTLifetime)

	claims := jwt.MapClaims{
		"sub": subject,
		"aud": sessionAudience,
		"iat": now.Unix(),
		"exp": expiresAt.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	signedToken, err := token.SignedString(i.privateKey)
	if err != nil {
		return "", time.Time{}, err
	}

	return signedToken, expiresAt, nil
}

// VerifySessionJWT validates a session token and returns its subject claim
// (the wallet credential). It enforces the EdDSA signing method, the "session"
// audience, expiry, and issued-at. Peer tokens (which lack the session
// audience) are rejected.
func (i *Issuer) VerifySessionJWT(tokenString string) (string, error) {
	token, err := jwt.Parse(
		tokenString,
		func(_ *jwt.Token) (any, error) {
			return i.publicKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithAudience(sessionAudience),
		jwt.WithExpirationRequired(),
		// Allow small amount of clock skew with freshly issued tokens
		jwt.WithLeeway(2*time.Second),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return "", err
	}
	// No explicit token.Valid check: jwt.Parse returns a non-nil error for any
	// validation failure and only leaves err nil on success, so reaching here
	// means the token is valid.

	// WithIssuedAt only validates iat when present; require it explicitly so a
	// token without iat is rejected (matching the mandatory exp claim).
	iat, err := token.Claims.GetIssuedAt()
	if err != nil {
		return "", err
	}
	if iat == nil {
		return "", errors.New("session token missing issued-at")
	}

	clientID, err := token.Claims.GetSubject()
	if err != nil {
		return "", err
	}
	if clientID == "" {
		return "", errors.New("session token missing subject")
	}

	return clientID, nil
}
