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

package crl

import (
	"context"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/ca"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Crl struct {
	ca                  *ca.Ca
	config              *config.Config
	db                  *database.Database
	logger              *slog.Logger
	nextScheduledUpdate time.Time
	needsUpdate         bool
	needsUpdateMutex    sync.Mutex
}

func New(
	cfg *config.Config,
	logger *slog.Logger,
	db *database.Database,
	ca *ca.Ca,
) (*Crl, error) {
	crl := &Crl{
		ca:     ca,
		config: cfg,
		db:     db,
		logger: logger,
	}
	if err := crl.updateConfigMap(); err != nil {
		return nil, fmt.Errorf("update CRL ConfigMap: %w", err)
	}
	// Schedule automatic ConfigMap updates
	crl.scheduleUpdateConfigMap()
	return crl, nil
}

func (c *Crl) SetNeedsUpdate() {
	c.needsUpdateMutex.Lock()
	c.needsUpdate = true
	c.needsUpdateMutex.Unlock()
}

func (c *Crl) k8sClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func (c *Crl) scheduleUpdateConfigMap() {
	tickChan := time.Tick(1 * time.Minute)
	c.nextScheduledUpdate = time.Now().Add(c.config.Crl.UpdateInterval)
	go func() {
		for {
			_, ok := <-tickChan
			if !ok {
				return
			}
			c.needsUpdateMutex.Lock()
			if time.Now().After(c.nextScheduledUpdate) || c.needsUpdate {
				if err := c.updateConfigMap(); err != nil {
					c.logger.Error(
						fmt.Sprintf(
							"failed to update CRL ConfigMap: %s",
							err,
						),
					)
				}
				if !c.needsUpdate {
					c.nextScheduledUpdate = c.nextScheduledUpdate.Add(
						c.config.Crl.UpdateInterval,
					)
				}
				c.needsUpdate = false
			}
			c.needsUpdateMutex.Unlock()
		}
	}()
}

func (c *Crl) updateConfigMap() error {
	// Build our revoked cert list from client expirations and manual list from config
	var revokedCerts []pkix.RevokedCertificate
	for _, serial := range c.config.Crl.RevokeSerials {
		serialBytes, err := hex.DecodeString(serial)
		if err != nil {
			return err
		}
		revokedCerts = append(
			revokedCerts,
			pkix.RevokedCertificate{
				SerialNumber:   new(big.Int).SetBytes(serialBytes),
				RevocationTime: c.config.Crl.RevokeTime,
			},
		)
	}
	expiredClients, err := c.db.ExpiredClients()
	if err != nil {
		return err
	}
	for _, client := range expiredClients {
		revokedCerts = append(
			revokedCerts,
			pkix.RevokedCertificate{
				SerialNumber: ca.ClientNameToSerialNumber(
					hex.EncodeToString(client.AssetName),
				),
				RevocationTime: client.Expiration,
			},
		)
	}
	crlData, err := c.ca.GenerateCRL(
		revokedCerts,
		time.Now(),
		time.Now().AddDate(1, 0, 0),
	)
	if err != nil {
		return err
	}
	client, err := c.k8sClient()
	if err != nil {
		return err
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.config.Crl.ConfigMapName,
		},
		Data: map[string]string{
			c.config.Crl.ConfigMapKey: string(crlData),
		},
	}
	// Check if ConfigMap already exists
	configMapExists := true
	_, err = client.CoreV1().
		ConfigMaps(c.config.Crl.ConfigMapNamespace).
		Get(context.TODO(), c.config.Crl.ConfigMapName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return fmt.Errorf("get ConfigMap: %w", err)
		}
		configMapExists = false
	}
	// Create/update ConfigMap
	if configMapExists {
		_, err = client.CoreV1().
			ConfigMaps(c.config.Crl.ConfigMapNamespace).
			Update(context.TODO(), configMap, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update ConfigMap: %w", err)
		}
		c.logger.Info(
			fmt.Sprintf(
				"updated CRL ConfigMap %s/%s",
				c.config.Crl.ConfigMapNamespace,
				c.config.Crl.ConfigMapName,
			),
		)
	} else {
		_, err = client.CoreV1().ConfigMaps(c.config.Crl.ConfigMapNamespace).Create(context.TODO(), configMap, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create ConfigMap: %w", err)
		}
		c.logger.Info(
			fmt.Sprintf(
				"created CRL ConfigMap %s/%s",
				c.config.Crl.ConfigMapNamespace,
				c.config.Crl.ConfigMapName,
			),
		)
	}
	return nil
}
