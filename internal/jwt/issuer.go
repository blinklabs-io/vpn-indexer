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
type Issuer struct {
	privateKey ed25519.PrivateKey
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

	return &Issuer{
		privateKey: ed25519Key,
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
