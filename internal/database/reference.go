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
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	referenceId = 1
)

type Reference struct {
	ID        uint `gorm:"primaryKey"`
	TxId      []byte
	OutputIdx int
	Prices    []ReferencePrice
	Regions   []ReferenceRegion
}

func (Reference) TableName() string {
	return "reference"
}

type ReferencePrice struct {
	ID          uint `gorm:"primaryKey"`
	ReferenceID uint
	Duration    int
	Price       int
}

type ReferenceRegion struct {
	ID          uint `gorm:"primaryKey"`
	ReferenceID uint
	Name        string
}

func (d *Database) ReferenceData() (Reference, error) {
	var ret Reference
	result := d.db.Where("id = ?", referenceId).
		Preload("Prices").
		Preload("Regions").
		First(&ret)
	if result.Error != nil {
		return ret, result.Error
	}
	return ret, nil
}

func (d *Database) UpdateReferenceData(
	txOutputId lcommon.TransactionInput,
	prices []ReferencePrice,
	regions []string,
) error {
	tmpRegions := make([]ReferenceRegion, 0, len(regions))
	for _, region := range regions {
		tmpRegions = append(
			tmpRegions,
			ReferenceRegion{
				Name: region,
			},
		)
	}
	tmpItem := Reference{
		ID:        referenceId,
		TxId:      txOutputId.Id().Bytes(),
		OutputIdx: int(txOutputId.Index()),
		Prices:    prices,
		Regions:   tmpRegions,
	}
	err := d.db.Transaction(func(tx *gorm.DB) error {
		if result := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ReferencePrice{}); result.Error != nil {
			return result.Error
		}
		if result := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&ReferenceRegion{}); result.Error != nil {
			return result.Error
		}
		result := tx.Session(&gorm.Session{FullSaveAssociations: true}).
			Clauses(
				clause.OnConflict{
					Columns:   []clause.Column{{Name: "id"}},
					UpdateAll: true,
				},
			).Create(&tmpItem)
		if result.Error != nil {
			return result.Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
