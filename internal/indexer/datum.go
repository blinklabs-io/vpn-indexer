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

package indexer

import (
	"errors"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type ClientDatum struct {
	cbor.StructAsArray
	Credential []byte
	Region     []byte
	Expiration uint
}

func (d *ClientDatum) UnmarshalCBOR(data []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(data, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 1 {
		return errors.New("invalid constructor")
	}
	type tClientDatum ClientDatum
	var tmp tClientDatum
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*d = ClientDatum(tmp)
	return nil
}

type ReferenceDatum struct {
	cbor.StructAsArray
	Prices  []ReferenceDatumPricing
	Regions [][]byte
}

func (d *ReferenceDatum) UnmarshalCBOR(data []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(data, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return errors.New("invalid constructor")
	}
	type tReferenceDatum ReferenceDatum
	var tmp tReferenceDatum
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*d = ReferenceDatum(tmp)
	return nil
}

type ReferenceDatumPricing struct {
	cbor.StructAsArray
	Duration int
	Price    int
}

func (p *ReferenceDatumPricing) UnmarshalCBOR(data []byte) error {
	var tmpConstr cbor.Constructor
	if _, err := cbor.Decode(data, &tmpConstr); err != nil {
		return err
	}
	if tmpConstr.Constructor() != 0 {
		return errors.New("invalid constructor")
	}
	type tReferenceDatumPricing ReferenceDatumPricing
	var tmp tReferenceDatumPricing
	if _, err := cbor.Decode(tmpConstr.FieldsCbor(), &tmp); err != nil {
		return err
	}
	*p = ReferenceDatumPricing(tmp)
	return nil
}
