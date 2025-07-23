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
	"fmt"
	"math/rand"

	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	maxCursorEntries = 50
)

type Cursor struct {
	ID   uint   `gorm:"primaryKey"`
	Hash []byte `gorm:"size:32"`
	Slot uint64
}

func (Cursor) TableName() string {
	return "cursor"
}

func (d *Database) AddCursorPoint(point ocommon.Point) error {
	tmpItem := Cursor{
		Hash: point.Hash,
		Slot: point.Slot,
	}
	if result := d.db.Create(&tmpItem); result.Error != nil {
		return result.Error
	}
	// Remove older cursor entries
	// We do this approximately 1% of the time to reduce DB writes
	if rand.Intn(100) == 0 {
		result := d.db.
			Where("id < (SELECT max(id) FROM cursor) - ?", maxCursorEntries).
			Delete(&Cursor{})
		if result.Error != nil {
			return fmt.Errorf(
				"failure removing cursor entries: %s",
				result.Error,
			)
		}
	}
	return nil
}

func (d *Database) GetCursorPoints() ([]ocommon.Point, error) {
	var cursorPoints []Cursor
	result := d.db.
		Order("id DESC").
		Find(&cursorPoints)
	if result.Error != nil {
		return nil, result.Error
	}
	ret := make([]ocommon.Point, len(cursorPoints))
	for i, tmpPoint := range cursorPoints {
		ret[i] = ocommon.Point{
			Hash: tmpPoint.Hash,
			Slot: tmpPoint.Slot,
		}
	}
	return ret, nil
}
