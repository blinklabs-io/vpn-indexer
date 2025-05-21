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
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
)

// Generated with:
// openssl genrsa -out rootCA.key 1024
var testCaCert = `
-----BEGIN CERTIFICATE-----
MIICADCCAWmgAwIBAgIUKGJCuwF+l6LbPeHQFqiEVHQMQ3kwDQYJKoZIhvcNAQEL
BQAwEjEQMA4GA1UEAwwHVGVzdCBDQTAeFw0yNTA1MjIxMzU1NThaFw0yODEwMDcx
MzU1NThaMBIxEDAOBgNVBAMMB1Rlc3QgQ0EwgZ8wDQYJKoZIhvcNAQEBBQADgY0A
MIGJAoGBAMJ2aA8dqU3TYg1YYJm/+eEqYdELFSw12/vViVnpmyr6lWdPpMZvs0a0
8tyXSAxFDfROD7smwejW4ZHP6uDlYvFOrdmVVceXyIFWMX9cTfV+Frcfr03VvP1r
LRdzw+gsrh/J5yYogOlVcyHNDJ6OYxrVLqlju7AFBH+MAgqTydb9AgMBAAGjUzBR
MB0GA1UdDgQWBBSX5PZIhtCq1999UtRHwqE0OtDvoTAfBgNVHSMEGDAWgBSX5PZI
htCq1999UtRHwqE0OtDvoTAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUA
A4GBACgl9vew/dQpChJGlsw3AOXT9gekqXG78D1rm237S3JDnxtLWpktjnZKD1w+
B6f57BfR6/FMhGAs1MmUD0g7lGmFekUFxH8TIppegcmsk4uotHNkLFtObe+rLYAE
5wMHPW7u2KEVu63Uv4MrWHi3sGxpUEak3efAQGOZLuKsyy3m
-----END CERTIFICATE-----
`

// Generated with:
// openssl req -x509 -new -nodes -key rootCA.key -sha256 -days 1234 -out rootCA.crt
var testCaKey = `
-----BEGIN PRIVATE KEY-----
MIICdwIBADANBgkqhkiG9w0BAQEFAASCAmEwggJdAgEAAoGBAMJ2aA8dqU3TYg1Y
YJm/+eEqYdELFSw12/vViVnpmyr6lWdPpMZvs0a08tyXSAxFDfROD7smwejW4ZHP
6uDlYvFOrdmVVceXyIFWMX9cTfV+Frcfr03VvP1rLRdzw+gsrh/J5yYogOlVcyHN
DJ6OYxrVLqlju7AFBH+MAgqTydb9AgMBAAECgYEArDnoIXMYrkfHsKAUNjeTnLtH
lLfnEZfF9D2D/zDpb2AtkCk2e1UUh0vdSEdn1Q4XtMaqIgvKc2hUsSpfEL24KQkw
eqo7w4avDw9vUTUMmgpoKPsa5awLL85k9N+aUfobIjHrG6ooY1FtZCam+3Q0sWF3
UniJMrjvsCfkKksGVMECQQDw3P3tGioE2TvnlbCHf3xPisqwf8VdZxtXGPF/H206
uFmKbvgbj9zucXGKJlIP6Xfe6yrTf5u9vKJNpoImdkIxAkEAzq7rcZWoTGLAya4K
6OkZa6YqGwHIoVT4hoLCM0sKhgDRKMoJYDmrhqqnMo/8AW1mMW1+/wvPLr5AR4IK
VpMCjQJAOVgz8GZBSMQ7eehujePxQbLGjPzujU1F+heLL3vY8pj/YHEJCu7WZ8KE
iKKU+QrZqi4NFSuVdbfaYGhbJjMTkQJAbPdbui6k5GDMM4hGyDTc6hxY5pQyKpyZ
ypD1wgU2LyAPJeoet1SwUfd23vl6a2Y6EqUf52dae9JiIVE2Eh6/oQJBANiu5C7Z
FL0gZq0wTto6HAgYEJnNlqLFR23/5146KsT8DMYh7t/WtVjXVBAbyrexsre5rZ2t
oP6A6uLgNPIYy94=
-----END PRIVATE KEY-----
`

// This is used to encrypt the key below
var testCaKeyEncPassphrase = `just a test`

// Generated with:
// openssl rsa -aes256 -in rootCA.key -out rootCA.enc.key
var testCaKeyEnc = `
-----BEGIN ENCRYPTED PRIVATE KEY-----
MIIC3TBXBgkqhkiG9w0BBQ0wSjApBgkqhkiG9w0BBQwwHAQIDMEoH1dJVbACAggA
MAwGCCqGSIb3DQIJBQAwHQYJYIZIAWUDBAEqBBBlSt8hZDgvubeBhYWwZbEFBIIC
gPMbcajWza1yoJ53xl3Qsx8BTd1KQXQ/JQ7wuSbI+ixjfYuIChY1uPXnhzmhDzIB
JZkrgJ/uJqLU56sNx5o1tPhTjHMe/VNCmTupuXEropN3ni2qwjG6i+58iful3JJQ
Tif45kRdF5CkPpsjWXF0uzgTQARw4z2lD90vprmgJw6iqWpAIBAmq16Nd5ud1PAT
OW+gkQKuqB6bz3yefxDZ2JVDIQFOrCvwqlHqZO3FFKn0UT56slufAGPprQTklAOp
ZUlwcdeish/1xEB6Lx1/8ae8apOVXil9FRC5WtTfdz9BRtbUZWpmWiPE6lSHmgUQ
TXGdueTBqcIjB69c48m8NH9Tq08D4xUbOJe/ZHRejAqlm4iy2ME77gDd64ZCa1n3
2MpOxFNT4tLJkA7EWbeVow1Nz9+RH4OYGCT2ylNHu2mYrtTNnf/IlvFO8D4GHjkJ
c89TmLaMEoeKO0Js7r4Ws/7XC/TSKeKlmorlPl6sCRrhyszUAion/QXYaQLIjpSx
gWsxGUIO/Pcfq+xfEzq6egjbv/AmwsPsNkzgDV9b38O355vc4xG9Qk1rUaXEVZcd
RNyuNLtcjb63WrLp+czVnZeedtmtip/uyx4F7mU9s6CbI8TdSsa+udcKn2zSYyUl
Vi6vXxx3PNnAXfcEil/Kzs5Tm+YX+dBfJTVswQu+5b7I7/kQ4AzxsNqG9hebgckR
tD/GvpKK7krZ7JFLS0baP5dZ6brfPY0WsHmUKjkCAO1tRopSOmwRVI5HUJYPdJOD
D8Y+8ILFdS+1uebj8ZbkWftQqQS4sHuZt0PxPzvU9LgqW+4aDTOcmz12VICzf0WP
0oM7b7Ig9sZw1I0Vpqalp8w=
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
