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

package ca

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
)

// Generated with:
// openssl genrsa -out rootCA.key 1024
var testCaCert = `
-----BEGIN CERTIFICATE-----
MIIClzCCAgCgAwIBAgIULdRPwP+Ue5oxNvgG6RjBFgEtovAwDQYJKoZIhvcNAQEL
BQAwVzELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDEQMA4GA1UEAwwHVGVzdCBDQTAeFw0y
NTA2MDUxODU1MTBaFw0yODEwMjExODU1MTBaMFcxCzAJBgNVBAYTAkFVMRMwEQYD
VQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBM
dGQxEDAOBgNVBAMMB1Rlc3QgQ0EwgZ8wDQYJKoZIhvcNAQEBBQADgY0AMIGJAoGB
AM25vK3+qvIdsYsdRBhoVnQa5pfG8UCODD1nGcFBujtRyNCZUQdyu0pX20LhRIUm
cTByGCOPsZxNr/kAK5mgXmOMWr/0dyyd9KHmeIFmdZCb8wGUI70XeTWIkXLYbffS
ttwaVV+dClb27FI7Pjzm3ZUMAJ7XifVpj0diVd94l81FAgMBAAGjYDBeMB0GA1Ud
DgQWBBRbpGrNjgwN/Jj8aLAoe+5AdtOapzAfBgNVHSMEGDAWgBRbpGrNjgwN/Jj8
aLAoe+5AdtOapzAPBgNVHRMBAf8EBTADAQH/MAsGA1UdDwQEAwIBBjANBgkqhkiG
9w0BAQsFAAOBgQAq+D287IeZ3R+s4beNyb0z9U4q+XmgZC2H0UtsoP+nDzvnq6EU
X5K0OZf3nKDQPV886jBYuqpXcYdk86ylQbPQJbvSzqGTxg/WTey4BPN51ojdYEvt
sQbsfCZK4tx5Q7FwfL9uk+tybKtEyrGKLr+JH07OwKhtQpYGoVtiD6U6nQ==
-----END CERTIFICATE-----
`

// Generated with:
// openssl req -x509 -new -nodes -key rootCA.key -sha256 -days 1234 -addext "keyUsage = cRLSign, keyCertSign" -out rootCA.crt
var testCaKey = `
-----BEGIN PRIVATE KEY-----
MIICdwIBADANBgkqhkiG9w0BAQEFAASCAmEwggJdAgEAAoGBAM25vK3+qvIdsYsd
RBhoVnQa5pfG8UCODD1nGcFBujtRyNCZUQdyu0pX20LhRIUmcTByGCOPsZxNr/kA
K5mgXmOMWr/0dyyd9KHmeIFmdZCb8wGUI70XeTWIkXLYbffSttwaVV+dClb27FI7
Pjzm3ZUMAJ7XifVpj0diVd94l81FAgMBAAECgYEArJlQO4qWUVuoQVbkcrXXEsIf
BOfcMJT8n+eILCPA41PSb3CyEtWnXNApHQtyOWPvQv32Up+UG9bx9K635cQua0U8
HVuJbm4GO6P+Q/I7cW8uIJPEdBKKbJwZ379F/APGBAP0RD5rJQ1Y65jP1Ii1yOsV
+Y2ayN7q00sIjkctbAECQQDvuEERGy3uIJGP5/YFkAEGuvV/QPyXYIE7TteFhzYr
nmU+U1qUEATBhJpGWn6AA1b4rz2PKbksap+5MfMDmGFhAkEA27J2b0P2FdOldy8u
OI+Tx5RFuz7dcjXV59fWnbRO9d0q8MDWDckZ9oqT2yLHQ5sZ1HMkQVDlhPnPc1/s
PBqiZQJAKjjCxReLbHCyEq2haHNnqt7NFJ/GnYby3BZT4YHiKaaZYHPf9Uoo/Ei1
v4R62WM9M0nyRr/rjIYvIbhJfC2foQJBAL7xAUw81eEsfE/0uohACSFZda2CurYr
ogiJJ6cS8dlv6oUqJCABG0aSNGUteeABKlbh56244HJNJ4bP5KJsR50CQEFT6XaA
rQ0aNyVXoRZrTewWsowzPAasprQhv9qUQPy14+iO9Nttfumge+r4Z6/oqYn9Fem2
xvIsZvJsUWLOo/c=
-----END PRIVATE KEY-----
`

// This is used to encrypt the key below
var testCaKeyEncPassphrase = `just a test`

// Generated with:
// openssl rsa -aes256 -in rootCA.key -out rootCA.enc.key
var testCaKeyEnc = `
-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIC3TBXBgkqhkiG9w0BBQ0wSjApBgkqhkiG9w0BBQwwHAQIloZ5IO6WRmwCAggA
MAwGCCqGSIb3DQIJBQAwHQYJYIZIAWUDBAEqBBDYgPbS34XyzQZqTR2otDU4BIIC
gB2ZlhscXgA/g3h5ou4AX4WIzcCVT65ZASELT6x7M+FsmfHSNIcqglcdjBycwvvR
ID1mYuPsZ595bevZX3IVN3jOu4TuN1RVyuYaR6I+oQtxqPJTUuL33z2IlBP4YBVz
276ntaZZxHEART8dJrZ5RUisJmyR/zNC61mom22cPC/zSem+LlEE+BPtd1BEyQ6W
vPU1NxVbX/H0P9BYXWKmN0/5DDwkY0JrBsry3pzIrPjBBro1dWyVsxhpvsCQQWr3
77depOw3dg2b2G/agKQSpIF8pmsLwFNAp0Qb6KcX0MAjrv2YALFTrKzz/rdF5Il2
4/LNBFAx3zwr8sbICpMgI1tg1VsTEaRRL8J26ZjiAHHtqEsSKkbP5YVAKLtxUrxv
i/OX1HaBkCb2OLfvtVjkVy7agtKMF7gyAjMpFLr8GmREYbev17/yGM9OvtGjQSjd
DTL1z6xFChroX57HAbKQXK3KAx4rZKcTlnLxmX3ePG1Vabh2dy3vSR6S2L/L9LGM
+S+Ut5f6RALbDlJN+Rg2inWdqFVvOGyiguTD4X1lp9fy8KUn+lXfQnfK8HbWepmE
Ds0maozFQCGbGGmKFkZ5Jvy7WuhUmvuUMZ6s25vfRKUyiTMfngyKCmWPAZKxXuOL
Xri0NZuKVcLIFoZPNEXLju4Sh3zXgQmwcbXXHD2G8qB7VV0gL7HAEdf5wfi2bepu
j16MyXFnruXcoTBXC+KXJBgX8rqTEC1XCEuWgLr7wQIiwpJeJrEv7/6g9/cKe7wG
WfPJDbvvj28T9lHm5dEj6ULtmS4l2HPqPGHu7QuaLfpYY5UsYCTXsIpR2uCS51RG
mjViPR4lTdzB/qtyf0fZkPk=
-----END ENCRYPTED PRIVATE KEY-----
`

func TestCaLoadCertKey(t *testing.T) {
	cfg := &config.Config{
		Ca: config.CaConfig{
			Cert: testCaCert,
			Key:  testCaKey,
		},
	}
	_, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating CA: %s", err)
	}
}

func TestCaLoadCertKeyEnc(t *testing.T) {
	cfg := &config.Config{
		Ca: config.CaConfig{
			Cert:       testCaCert,
			Key:        testCaKeyEnc,
			Passphrase: testCaKeyEncPassphrase,
		},
	}
	_, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating CA: %s", err)
	}
}

func TestCaCreateClient(t *testing.T) {
	testClientName := "test-client"
	expectedCertSerial := "1257c92663bc26742ef2230f60e585466f48e514"
	cfg := &config.Config{
		Ca: config.CaConfig{
			Cert: testCaCert,
			Key:  testCaKey,
		},
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating CA: %s", err)
	}
	client, err := c.GenerateClientCert(testClientName)
	if err != nil {
		t.Fatalf("unexpected error generating client cert: %s", err)
	}
	if client.CaCert != testCaCert {
		t.Fatalf("did not get expected CA cert\n     got: %s\n  wanted: %s", client.CaCert, testCaCert)
	}
	pemBlock, _ := pem.Decode([]byte(client.Cert))
	if pemBlock == nil {
		t.Fatal("unexpected failure decoding PEM data")
	}
	tmpCert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		t.Fatalf("unexpected error parsing generated certificate: %s", err)
	}
	serialHex := hex.EncodeToString(tmpCert.SerialNumber.Bytes())
	if serialHex != expectedCertSerial {
		t.Fatalf("did not get expected cert serial: got %s, wanted %s", serialHex, expectedCertSerial)
	}
}

func TestCaCreateClientEncKey(t *testing.T) {
	testClientName := "test-client"
	expectedCertSerial := "1257c92663bc26742ef2230f60e585466f48e514"
	cfg := &config.Config{
		Ca: config.CaConfig{
			Cert:       testCaCert,
			Key:        testCaKeyEnc,
			Passphrase: testCaKeyEncPassphrase,
		},
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating CA: %s", err)
	}
	client, err := c.GenerateClientCert(testClientName)
	if err != nil {
		t.Fatalf("unexpected error generating client cert: %s", err)
	}
	if client.CaCert != testCaCert {
		t.Fatalf("did not get expected CA cert\n     got: %s\n  wanted: %s", client.CaCert, testCaCert)
	}
	pemBlock, _ := pem.Decode([]byte(client.Cert))
	if pemBlock == nil {
		t.Fatal("unexpected failure decoding PEM data")
	}
	tmpCert, err := x509.ParseCertificate(pemBlock.Bytes)
	if err != nil {
		t.Fatalf("unexpected error parsing generated certificate: %s", err)
	}
	serialHex := hex.EncodeToString(tmpCert.SerialNumber.Bytes())
	if serialHex != expectedCertSerial {
		t.Fatalf("did not get expected cert serial: got %s, wanted %s", serialHex, expectedCertSerial)
	}
}

func TestCaGenerateCRL(t *testing.T) {
	testRevokedCerts := []pkix.RevokedCertificate{
		{
			SerialNumber:   big.NewInt(123456789),
			RevocationTime: time.Date(2025, 6, 5, 12, 34, 56, 0, time.UTC),
		},
		{
			SerialNumber:   big.NewInt(234567890),
			RevocationTime: time.Date(2025, 6, 5, 12, 12, 12, 0, time.UTC),
		},
	}
	testIssuedTime := time.Date(2025, 6, 5, 18, 31, 14, 0, time.UTC)
	testExpireTime := testIssuedTime.AddDate(1, 0, 0)
	// NOTE: this content was verified with 'openssl crl -text -noout'
	expectedCrl := strings.TrimSpace(`
-----BEGIN X509 CRL-----
MIIBhzCB8QIBATANBgkqhkiG9w0BAQsFADBXMQswCQYDVQQGEwJBVTETMBEGA1UE
CAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50ZXJuZXQgV2lkZ2l0cyBQdHkgTHRk
MRAwDgYDVQQDDAdUZXN0IENBFw0yNTA2MDUxODMxMTRaFw0yNjA2MDUxODMxMTRa
MC4wFQIEB1vNFRcNMjUwNjA1MTIzNDU2WjAVAgQN+zjSFw0yNTA2MDUxMjEyMTJa
oDYwNDAfBgNVHSMEGDAWgBRbpGrNjgwN/Jj8aLAoe+5AdtOapzARBgNVHRQECgII
GEY5FntB9AAwDQYJKoZIhvcNAQELBQADgYEAvpLSbPuBJ9VbGssbbu0JdBpV/crR
x1sVyJ06M1eriu9Y62cV6gJpFihn4Iv104XNlPFXORqd8SQQNa8ljD2KXf6UCAYp
/kcP8k6e0xkLar6456IpBv9Wjbbi+4CCpK8sX8fdGCmhyDlrBzD9ix0W/TURq3xK
oc9CHTH5lsWcmzc=
-----END X509 CRL-----
`)
	cfg := &config.Config{
		Ca: config.CaConfig{
			Cert:       testCaCert,
			Key:        testCaKeyEnc,
			Passphrase: testCaKeyEncPassphrase,
		},
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating CA: %s", err)
	}
	crlBytes, err := c.GenerateCRL(testRevokedCerts, testIssuedTime, testExpireTime)
	if err != nil {
		t.Fatalf("unexpected error generating CRL: %s", err)
	}
	tmpCrl := strings.TrimSpace(string(crlBytes))
	if tmpCrl != expectedCrl {
		t.Fatalf("did not get expected CRL content\n     got:\n%s\n  wanted:\n%s", tmpCrl, expectedCrl)
	}
}
