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

package txbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Salvionied/apollo/serialization/Amount"
	"github.com/Salvionied/apollo/serialization/TransactionInput"
	"github.com/Salvionied/apollo/serialization/UTxO"
	"github.com/Salvionied/apollo/serialization/Value"
	"github.com/Salvionied/apollo/txBuilding/Backend/OgmiosChainContext"
	"github.com/SundaeSwap-finance/kugo"
	"github.com/SundaeSwap-finance/ogmigo/v6"
	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
)

const (
	defaultKupoTimeout = 1 * time.Second
)

var systemStart *time.Time

func apolloBackend() (*OgmiosChainContext.OgmiosChainContext, error) {
	cfg := config.GetConfig()
	ogmiosClient := OgmiosClient()
	kupoClient := kugo.New(
		kugo.WithEndpoint(cfg.TxBuilder.KupoUrl),
		kugo.WithTimeout(defaultKupoTimeout),
		kugo.WithLogger(ogmigo.NopLogger),
	)
	occ := OgmiosChainContext.NewOgmiosChainContext(ogmiosClient, kupoClient)
	return &occ, nil
}

func OgmiosClient() *ogmigo.Client {
	cfg := config.GetConfig()
	ogmiosClient := ogmigo.New(
		ogmigo.WithEndpoint(cfg.TxBuilder.OgmiosUrl),
	)
	return ogmiosClient
}

// It clears the cached Shelley genesis start time
func ResetCachedSystemStart() {
	systemStart = nil
}

func ogmiosSystemStart(ogmios *ogmigo.Client) (time.Time, error) {
	// Return cached system start
	if systemStart != nil {
		return *systemStart, nil
	}
	// Get system start from Shelley genesis config
	genesisConfigRaw, err := ogmios.GenesisConfig(
		context.Background(),
		"shelley",
	)
	if err != nil {
		return *systemStart, err
	}
	var tmpGenesisConfig struct {
		StartTime time.Time `json:"startTime"`
	}
	if err := json.Unmarshal(genesisConfigRaw, &tmpGenesisConfig); err != nil {
		return *systemStart, err
	}
	systemStart = &(tmpGenesisConfig.StartTime)
	return *systemStart, nil
}

func inputRefFromString(ref string) (lcommon.TransactionInput, error) {
	var refInput shelley.ShelleyTransactionInput
	var tmpTxId []byte
	_, err := fmt.Sscanf(
		ref,
		"%x#%d",
		&tmpTxId,
		&refInput.OutputIndex,
	)
	refInput.TxId = lcommon.Blake2b256(tmpTxId)
	if err != nil {
		return nil, fmt.Errorf("parse script ref input: %w", err)
	}
	return refInput, nil
}

func chooseInputUtxos(
	availableUtxos []UTxO.UTxO,
	neededAmount int,
) ([]UTxO.UTxO, error) {
	var ret []UTxO.UTxO
	// The below code is adapted from Apollo's own UTxO selection code
	selectedAmount := Value.Value{}
	requestedAmount := Value.Value{
		Am:        Amount.Amount{},
		Coin:      int64(neededAmount),
		HasAssets: false,
	}
	for !selectedAmount.Greater(
		requestedAmount.Add(
			Value.Value{Am: Amount.Amount{}, Coin: 1_000_000, HasAssets: false},
		),
	) {
		if len(availableUtxos) == 0 {
			return nil, errors.New("not enough funds")
		}
		utxo := availableUtxos[0]
		// Discard inputs with assets for simplicity
		if utxo.Output.GetValue().HasAssets {
			availableUtxos = availableUtxos[1:]
			continue
		}
		ret = append(ret, utxo)
		selectedAmount = selectedAmount.Add(utxo.Output.GetValue())
		availableUtxos = availableUtxos[1:]
	}
	return ret, nil
}

func clientIdFromInput(
	input TransactionInput.TransactionInput,
) ([]byte, error) {
	tmpData := cbor.NewConstructor(
		0,
		cbor.IndefLengthList{
			input.TransactionId,
			input.Index,
		},
	)
	hashData, err := cbor.Encode(tmpData)
	if err != nil {
		return nil, err
	}
	hash := lcommon.Blake2b256Hash(hashData)
	return hash.Bytes(), nil
}

func determinePlanSelection(
	refData database.Reference,
	price int,
	duration int,
) (int, error) {
	for idx, tmpPrice := range refData.Prices {
		if tmpPrice.Price != price {
			continue
		}
		if tmpPrice.Duration != duration {
			continue
		}
		return idx, nil
	}
	return 0, errors.New("selection not found")
}

// InputValidationError is a custom error type representing input validation errors
type InputValidationError struct {
	msg string
}

func NewInputValidationError(msg string) error {
	return InputValidationError{
		msg: msg,
	}
}

func (e InputValidationError) Error() string {
	return e.msg
}
