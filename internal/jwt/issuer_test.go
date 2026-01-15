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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// generateTestEd25519Key creates a test Ed25519 private key PEM file
// and returns the path to the file
func generateTestEd25519Key(t *testing.T) (string, ed25519.PublicKey) {
	t.Helper()

	// Generate Ed25519 key pair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate Ed25519 key: %v", err)
	}

	// Marshal private key to PKCS8 format
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}

	// Create PEM block
	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	}

	// Write to temp file
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "ed25519.key")
	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("failed to create key file: %v", err)
	}
	defer func() { _ = keyFile.Close() }()

	if err := pem.Encode(keyFile, pemBlock); err != nil {
		t.Fatalf("failed to write PEM data: %v", err)
	}

	return keyPath, pubKey
}

func TestNewIssuerWithValidKey(t *testing.T) {
	keyPath, _ := generateTestEd25519Key(t)

	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	if issuer == nil {
		t.Fatal("expected issuer to not be nil")
	}

	if issuer.privateKey == nil {
		t.Fatal("expected private key to not be nil")
	}
}

func TestNewIssuerWithMissingFile(t *testing.T) {
	_, err := NewIssuer("/nonexistent/path/to/key.pem")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestNewIssuerWithInvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "invalid.key")

	// Write invalid PEM data
	if err := os.WriteFile(keyPath, []byte("not valid pem data"), 0600); err != nil {
		t.Fatalf("failed to write invalid key file: %v", err)
	}

	_, err := NewIssuer(keyPath)
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
}

func TestNewIssuerWithWrongKeyType(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "rsa.key")

	// Generate a real RSA key to test the key type check
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// Marshal RSA private key to PKCS8 format
	rsaKeyBytes, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	if err != nil {
		t.Fatalf("failed to marshal RSA key: %v", err)
	}

	// Write a valid PKCS8 PEM block with RSA key (wrong type for Ed25519)
	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: rsaKeyBytes,
	}

	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("failed to create key file: %v", err)
	}
	defer func() { _ = keyFile.Close() }()

	if err := pem.Encode(keyFile, pemBlock); err != nil {
		t.Fatalf("failed to write PEM data: %v", err)
	}

	_, err = NewIssuer(keyPath)
	if err == nil {
		t.Fatal(
			"expected error for wrong key type (RSA instead of Ed25519), got nil",
		)
	}
}

func TestIssuePeerJWTGeneratesValidToken(t *testing.T) {
	keyPath, pubKey := generateTestEd25519Key(t)

	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	testPubkey := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijk="
	testAllowedIP := "10.8.0.42"

	tokenString, err := issuer.IssuePeerJWT(testPubkey, testAllowedIP)
	if err != nil {
		t.Fatalf("unexpected error issuing JWT: %v", err)
	}

	if tokenString == "" {
		t.Fatal("expected non-empty token string")
	}

	// Parse and verify the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			t.Fatalf("unexpected signing method: %v", token.Header["alg"])
		}
		return pubKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	if !token.Valid {
		t.Fatal("token is not valid")
	}
}

func TestIssuePeerJWTHasCorrectClaims(t *testing.T) {
	keyPath, pubKey := generateTestEd25519Key(t)

	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	testPubkey := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijk="
	testAllowedIP := "10.8.0.42"

	beforeIssue := time.Now().Unix()
	tokenString, err := issuer.IssuePeerJWT(testPubkey, testAllowedIP)
	if err != nil {
		t.Fatalf("unexpected error issuing JWT: %v", err)
	}
	afterIssue := time.Now().Unix()

	// Parse the token to extract claims
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return pubKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to cast claims to MapClaims")
	}

	// Verify subject claim
	sub, ok := claims["sub"].(string)
	if !ok || sub != "wg_peer" {
		t.Fatalf("expected sub claim to be 'wg_peer', got %v", claims["sub"])
	}

	// Verify pubkey claim
	pk, ok := claims["pubkey"].(string)
	if !ok || pk != testPubkey {
		t.Fatalf(
			"expected pubkey claim to be %q, got %v",
			testPubkey,
			claims["pubkey"],
		)
	}

	// Verify allowed_ip claim
	ip, ok := claims["allowed_ip"].(string)
	if !ok || ip != testAllowedIP {
		t.Fatalf(
			"expected allowed_ip claim to be %q, got %v",
			testAllowedIP,
			claims["allowed_ip"],
		)
	}

	// Verify iat claim (issued at)
	iat, ok := claims["iat"].(float64)
	if !ok {
		t.Fatal("expected iat claim to be a number")
	}
	iatUnix := int64(iat)
	if iatUnix < beforeIssue || iatUnix > afterIssue {
		t.Fatalf(
			"expected iat to be between %d and %d, got %d",
			beforeIssue,
			afterIssue,
			iatUnix,
		)
	}

	// Verify exp claim (expiration)
	exp, ok := claims["exp"].(float64)
	if !ok {
		t.Fatal("expected exp claim to be a number")
	}
	expUnix := int64(exp)

	// Expiry should be PeerJWTLifetime after iat
	expectedExp := iatUnix + int64(PeerJWTLifetime.Seconds())
	if expUnix != expectedExp {
		t.Fatalf(
			"expected exp to be %d (iat + %d), got %d",
			expectedExp,
			int64(PeerJWTLifetime.Seconds()),
			expUnix,
		)
	}
}

func TestIssuePeerJWTExpiry(t *testing.T) {
	keyPath, pubKey := generateTestEd25519Key(t)

	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	testPubkey := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijk="
	testAllowedIP := "10.8.0.42"

	tokenString, err := issuer.IssuePeerJWT(testPubkey, testAllowedIP)
	if err != nil {
		t.Fatalf("unexpected error issuing JWT: %v", err)
	}

	// Parse the token to extract claims
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return pubKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to cast claims to MapClaims")
	}

	iat, _ := claims["iat"].(float64)
	exp, _ := claims["exp"].(float64)

	// Verify expiry is exactly PeerJWTLifetime after issued time
	expiryDuration := int64(exp) - int64(iat)
	expectedDuration := int64(PeerJWTLifetime.Seconds())
	if expiryDuration != expectedDuration {
		t.Fatalf(
			"expected JWT expiry to be %d seconds, got %d seconds",
			expectedDuration,
			expiryDuration,
		)
	}
}
