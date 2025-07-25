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

import "time"

type Client struct {
	ID         uint   `gorm:"primaryKey"`
	AssetName  []byte `gorm:"index"`
	Expiration time.Time
	Credential []byte
	Region     string
}

func (Client) TableName() string {
	return "client"
}

func (d *Database) AddClient(
	assetName []byte,
	expiration time.Time,
	credential []byte,
	region string,
) error {
	tmpItem := Client{
		AssetName:  assetName,
		Expiration: expiration,
		Credential: credential,
		Region:     region,
	}
	if result := d.db.Create(&tmpItem); result.Error != nil {
		return result.Error
	}
	return nil
}

func (d *Database) ExpiredClients() ([]Client, error) {
	var ret []Client
	result := d.db.Where("expiration < datetime('now')").
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
