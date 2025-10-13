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
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

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

// referenceJSON is a CLI/JSON view for Reference without touching db.
type referenceJSON struct {
	// txId is hex-encoded & decodes to Reference.TxId ([]byte)
	TxId      string           `json:"txId"`
	OutputIdx int              `json:"outputIdx"`
	Prices    []referencePrice `json:"prices"`
	Regions   []string         `json:"regions"`
}

type referencePrice struct {
	Duration int `json:"duration"`
	Price    int `json:"price"`
}

func ReferenceFromJSON(r io.Reader) (Reference, error) {
	var in referenceJSON
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return Reference{}, fmt.Errorf("parse refdata json: %w", err)
	}
	txid, err := hex.DecodeString(in.TxId)
	if err != nil {
		return Reference{}, fmt.Errorf("refdata.txId hex: %w", err)
	}

	prices := make([]ReferencePrice, 0, len(in.Prices))
	for _, p := range in.Prices {
		prices = append(prices, ReferencePrice{
			Duration: p.Duration,
			Price:    p.Price,
		})
	}
	regions := make([]ReferenceRegion, 0, len(in.Regions))
	for _, name := range in.Regions {
		regions = append(regions, ReferenceRegion{Name: name})
	}

	return Reference{
		TxId:      txid,
		OutputIdx: in.OutputIdx,
		Prices:    prices,
		Regions:   regions,
	}, nil
}

// It encodes a Reference to JSON using a stable schema.
func WriteReferenceJSON(w io.Writer, ref Reference) error {
	out := referenceJSON{
		TxId:      hex.EncodeToString(ref.TxId),
		OutputIdx: ref.OutputIdx,
		Prices:    make([]referencePrice, 0, len(ref.Prices)),
		Regions:   make([]string, 0, len(ref.Regions)),
	}
	for _, p := range ref.Prices {
		out.Prices = append(out.Prices, referencePrice{
			Duration: p.Duration,
			Price:    p.Price,
		})
	}
	for _, r := range ref.Regions {
		out.Regions = append(out.Regions, r.Name)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func LoadFromChain(ctx context.Context, ogmiosURL, kupoURL, scriptAddress string) (Reference, error) {
	// TODO: Need to complete implementing this part.
	return Reference{}, errors.New("on-chain refdata loading is disabled for now; use --refdata")
}
