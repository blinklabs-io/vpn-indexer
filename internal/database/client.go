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

package database

import (
	"time"

	"gorm.io/gorm/clause"
)

type Client struct {
	ID            uint   `gorm:"primaryKey"`
	AssetName     []byte `gorm:"uniqueIndex"`
	Expiration    time.Time
	Credential    []byte
	Region        string
	TxHash        []byte
	TxOutputIndex uint
}

func (Client) TableName() string {
	return "client"
}

func (d *Database) AddClient(
	assetName []byte,
	expiration time.Time,
	credential []byte,
	region string,
	txHash []byte,
	txOutputIndex uint,
) error {
	tmpItem := Client{
		AssetName:     assetName,
		Expiration:    expiration,
		Credential:    credential,
		Region:        region,
		TxHash:        txHash,
		TxOutputIndex: txOutputIndex,
	}
	onConflict := clause.OnConflict{
		Columns:   []clause.Column{{Name: "asset_name"}},
		UpdateAll: true,
	}
	if result := d.db.Clauses(onConflict).Create(&tmpItem); result.Error != nil {
		return result.Error
	}
	return nil
}

func (d *Database) ExpiredClients() ([]Client, error) {
	var ret []Client
	result := d.db.
		Where(
			"expiration < datetime('now') AND region == ?",
			d.config.Vpn.Region,
		).
		Order("expiration").
		Find(&ret)
	if result.Error != nil {
		return nil, result.Error
	}
	return ret, nil
}

func (d *Database) ClientsByCredential(
	paymentKeyHash []byte,
) ([]Client, error) {
	var ret []Client
	result := d.db.Where("credential = ?", paymentKeyHash).
		Order("id").
		Find(&ret)
	if result.Error != nil {
		return nil, result.Error
	}
	return ret, nil
}

func (d *Database) ClientByAssetName(assetName []byte) (Client, error) {
	var ret Client
	result := d.db.Where("asset_name = ?", assetName).First(&ret)
	if result.Error != nil {
		return ret, result.Error
	}
	return ret, nil
}
