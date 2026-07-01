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
// and returns the path to the file along with the corresponding public key.
func generateTestEd25519Key(t *testing.T) (string, ed25519.PublicKey) {
	t.Helper()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate Ed25519 key: %v", err)
	}

	return writeTestKeyFile(t, privKey), pubKey
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

// writeTestKeyFile marshals an Ed25519 private key to a PKCS8 PEM file and
// returns its path.
func writeTestKeyFile(t *testing.T, privKey ed25519.PrivateKey) string {
	t.Helper()
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	keyPath := filepath.Join(t.TempDir(), "ed25519.key")
	keyFile, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("failed to create key file: %v", err)
	}
	defer func() { _ = keyFile.Close() }()
	if err := pem.Encode(
		keyFile,
		&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyBytes},
	); err != nil {
		t.Fatalf("failed to write PEM data: %v", err)
	}
	return keyPath
}

func TestSessionJWTRoundTrip(t *testing.T) {
	keyPath, _ := generateTestEd25519Key(t)
	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	clientID := "0123456789abcdef"
	token, _, err := issuer.IssueSessionJWT(clientID)
	if err != nil {
		t.Fatalf("unexpected error issuing session token: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token string")
	}

	got, err := issuer.VerifySessionJWT(token)
	if err != nil {
		t.Fatalf("unexpected error verifying session token: %v", err)
	}
	if got != clientID {
		t.Fatalf("expected client ID %q, got %q", clientID, got)
	}
}

func TestSessionJWTHasCorrectClaims(t *testing.T) {
	keyPath, pubKey := generateTestEd25519Key(t)
	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	clientID := "0123456789abcdef"
	tokenString, expiresAt, err := issuer.IssueSessionJWT(clientID)
	if err != nil {
		t.Fatalf("unexpected error issuing session token: %v", err)
	}

	token, err := jwt.Parse(tokenString, func(_ *jwt.Token) (any, error) {
		return pubKey, nil
	})
	if err != nil {
		t.Fatalf("failed to parse token: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("failed to cast claims to MapClaims")
	}

	if sub, _ := claims["sub"].(string); sub != clientID {
		t.Fatalf("expected sub %q, got %v", clientID, claims["sub"])
	}
	if aud, _ := claims["aud"].(string); aud != sessionAudience {
		t.Fatalf("expected aud %q, got %v", sessionAudience, claims["aud"])
	}
	iat, _ := claims["iat"].(float64)
	exp, _ := claims["exp"].(float64)
	if got := int64(exp) - int64(iat); got != int64(SessionJWTLifetime.Seconds()) {
		t.Fatalf(
			"expected lifetime %d seconds, got %d",
			int64(SessionJWTLifetime.Seconds()),
			got,
		)
	}
	// The returned expiry must exactly match the token's exp claim so the
	// API's reported expires_at can never disagree with the token.
	if expiresAt.Unix() != int64(exp) {
		t.Fatalf(
			"returned expiry %d does not match exp claim %d",
			expiresAt.Unix(),
			int64(exp),
		)
	}
}

func TestVerifySessionJWTRejectsPeerToken(t *testing.T) {
	keyPath, _ := generateTestEd25519Key(t)
	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	// A peer token lacks the "session" audience and must be rejected.
	peerToken, err := issuer.IssuePeerJWT("somepubkey", "10.8.0.2")
	if err != nil {
		t.Fatalf("unexpected error issuing peer token: %v", err)
	}
	if _, err := issuer.VerifySessionJWT(peerToken); err == nil {
		t.Fatal("expected peer token to be rejected as a session token")
	}
}

func TestVerifySessionJWTRejectsExpired(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	issuer, err := NewIssuer(writeTestKeyFile(t, privKey))
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	// Craft an already-expired session token signed by the issuer's key.
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": "0123456789abcdef",
		"aud": sessionAudience,
		"iat": now.Add(-2 * time.Hour).Unix(),
		"exp": now.Add(-1 * time.Hour).Unix(),
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).
		SignedString(privKey)
	if err != nil {
		t.Fatalf("failed to sign expired token: %v", err)
	}

	if _, err := issuer.VerifySessionJWT(expired); err == nil {
		t.Fatal("expected expired session token to be rejected")
	}
}

func TestVerifySessionJWTRejectsMissingExp(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	issuer, err := NewIssuer(writeTestKeyFile(t, privKey))
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	// A token without an exp claim must be rejected (no non-expiring tokens).
	claims := jwt.MapClaims{
		"sub": "0123456789abcdef",
		"aud": sessionAudience,
		"iat": time.Now().Unix(),
	}
	noExp, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).
		SignedString(privKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	if _, err := issuer.VerifySessionJWT(noExp); err == nil {
		t.Fatal("expected session token without exp to be rejected")
	}
}

func TestVerifySessionJWTRejectsMissingIssuedAt(t *testing.T) {
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	issuer, err := NewIssuer(writeTestKeyFile(t, privKey))
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	// A token without an iat claim must be rejected.
	claims := jwt.MapClaims{
		"sub": "0123456789abcdef",
		"aud": sessionAudience,
		"exp": time.Now().Add(SessionJWTLifetime).Unix(),
	}
	noIat, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).
		SignedString(privKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	if _, err := issuer.VerifySessionJWT(noIat); err == nil {
		t.Fatal("expected session token without iat to be rejected")
	}
}

func TestVerifySessionJWTRejectsWrongKey(t *testing.T) {
	keyPath, _ := generateTestEd25519Key(t)
	issuer, err := NewIssuer(keyPath)
	if err != nil {
		t.Fatalf("unexpected error creating issuer: %v", err)
	}

	// Sign a valid-looking session token with a different key.
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": "0123456789abcdef",
		"aud": sessionAudience,
		"iat": now.Unix(),
		"exp": now.Add(SessionJWTLifetime).Unix(),
	}
	forged, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).
		SignedString(otherPriv)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	if _, err := issuer.VerifySessionJWT(forged); err == nil {
		t.Fatal("expected token signed with wrong key to be rejected")
	}
}
