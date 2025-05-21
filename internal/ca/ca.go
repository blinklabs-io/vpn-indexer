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
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/youmark/pkcs8"
	"golang.org/x/crypto/blake2b"
)

type Ca struct {
	caCert    *x509.Certificate
	caKey     any
	caCertPem []byte
}

type ClientCert struct {
	CaCert string
	Cert   string
	Key    string
}

func New(cfg *config.Config) (*Ca, error) {
	c := &Ca{}
	// Certificate
	if err := c.loadCert(cfg); err != nil {
		return nil, err
	}
	// Private key
	if err := c.loadKey(cfg); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Ca) loadCert(cfg *config.Config) error {
	var certData []byte
	if cfg.Ca.Cert != "" {
		certData = []byte(cfg.Ca.Cert)
	} else if cfg.Ca.CertFile != "" {
		pemData, err := os.ReadFile(cfg.Ca.CertFile)
		if err != nil {
			return err
		}
		certData = pemData
	} else {
		return errors.New("no CA certificate provided")
	}
	// Decode certificate PEM data
	decCert, _ := pem.Decode(certData)
	if decCert == nil {
		return errors.New("failed to decode PEM data")
	}
	tmpCert, err := x509.ParseCertificate(decCert.Bytes)
	if err != nil {
		return err
	}
	c.caCert = tmpCert
	c.caCertPem = certData
	return nil
}

func (c *Ca) loadKey(cfg *config.Config) error {
	var err error
	// Passphrase
	var passphrase []byte
	if cfg.Ca.Passphrase != "" {
		passphrase = []byte(cfg.Ca.Passphrase)
	} else if cfg.Ca.PassphraseFile != "" {
		passphrase, err = os.ReadFile(cfg.Ca.PassphraseFile)
		if err != nil {
			return err
		}
	}
	// Private key
	var keyData []byte
	if cfg.Ca.Key != "" {
		keyData = []byte(cfg.Ca.Key)
	} else if cfg.Ca.KeyFile != "" {
		pemData, err := os.ReadFile(cfg.Ca.KeyFile)
		if err != nil {
			return err
		}
		keyData = pemData
	} else {
		return errors.New("no CA private key provided")
	}
	// Decode key PEM data
	decKey, _ := pem.Decode(keyData)
	if decKey == nil {
		return errors.New("failed to decode PEM data")
	}
	key, err := pkcs8.ParsePKCS8PrivateKey(decKey.Bytes, passphrase)
	if err != nil {
		return err
	}
	c.caKey = key
	return nil
}

func (c *Ca) GenerateClientCert(clientName string) (*ClientCert, error) {
	// Hash client name using blake2b-160 to use as cert serial number
	hasher, _ := blake2b.New(20, nil)
	hasher.Write([]byte(clientName))
	clientNameHash := hasher.Sum(nil)
	// Cert template
	cert := &x509.Certificate{
		SerialNumber: new(big.Int).SetBytes(clientNameHash),
		Subject: pkix.Name{
			CommonName: clientName,
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().AddDate(10, 0, 0), // 10 years
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,
	}
	// Generate random key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, cert, c.caCert, &privKey.PublicKey, c.caKey)
	if err != nil {
		return nil, err
	}
	// Encode cert to PEM
	certPem := bytes.NewBuffer(nil)
	err = pem.Encode(
		certPem,
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: certBytes,
		},
	)
	if err != nil {
		return nil, err
	}
	// Encode private key to PEM
	keyPem := bytes.NewBuffer(nil)
	err = pem.Encode(
		keyPem,
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privKey),
		},
	)
	if err != nil {
		return nil, err
	}
	ret := &ClientCert{
		CaCert: string(c.caCertPem),
		Cert:   certPem.String(),
		Key:    keyPem.String(),
	}
	return ret, nil
}
