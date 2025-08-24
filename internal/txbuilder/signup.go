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
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Salvionied/apollo"
	"github.com/Salvionied/apollo/serialization"
	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/SundaeSwap-finance/ogmigo/v6"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
)

func BuildSignupTx(
	db *database.Database,
	clientAddress string,
	price int,
	duration int,
	region string,
) ([]byte, []byte, error) {
	cfg := config.GetConfig()
	cc, err := apolloBackend()
	if err != nil {
		return nil, nil, err
	}
	clientAddr, err := serAddress.DecodeAddress(clientAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("client address: %w", err)
	}
	scriptAddress, err := serAddress.DecodeAddress(cfg.Indexer.ScriptAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("script address: %w", err)
	}
	scriptHash := scriptAddress.PaymentPart
	providerAddress, err := serAddress.DecodeAddress(
		cfg.TxBuilder.ProviderAddress,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("provider address: %w", err)
	}
	// Lookup reference data
	refData, err := db.ReferenceData()
	if err != nil {
		return nil, nil, fmt.Errorf("reference data: %w", err)
	}
	// Parse script ref
	scriptRef, err := inputRefFromString(cfg.TxBuilder.ScriptRefInput)
	if err != nil {
		return nil, nil, err
	}
	// Get available UTxOs from user's wallet
	availableUtxos, err := cc.Utxos(clientAddr)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"lookup UTxOs for address: %s: %w",
			clientAddr.String(),
			err,
		)
	}
	// Choose input UTxOs from user's wallet
	inputUtxos, err := chooseInputUtxos(availableUtxos, price+5_000_000)
	if err != nil {
		return nil, nil, fmt.Errorf("choose input UTxOs: %w", err)
	}
	if len(inputUtxos) == 0 {
		return nil, nil, errors.New("no input UTxOs found")
	}
	// Determine client ID from first selected input UTxO
	clientId := clientIdFromInput(inputUtxos[0].Input)
	// Determine plan selection ID from price/duration
	selectionId, err := determinePlanSelection(refData, price, duration)
	if err != nil {
		return nil, nil, fmt.Errorf("determine plan selection: %w", err)
	}
	// Get last known slot
	curSlot, err := cc.LastBlockSlot()
	if err != nil {
		return nil, nil, fmt.Errorf("query latest block slot: %w", err)
	}
	// Calculate time for last known slot
	ogmios := OgmiosClient()
	systemStart, err := ogmiosSystemStart(ogmios)
	if err != nil {
		return nil, nil, fmt.Errorf("query system start: %w", err)
	}
	eraHistory, err := ogmios.EraSummaries(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("query era summaries: %w", err)
	}
	curSlotTime := systemStart.Add(
		time.Duration(
			ogmigo.SlotToElapsedMilliseconds(
				eraHistory,
				uint64(curSlot),
			),
		) * time.Millisecond,
	)
	// Configure transaction builder
	apollob := apollo.New(cc)
	apollob, err = apollob.
		SetWalletFromBech32(clientAddress).
		SetWalletAsChangeAddress()
	if err != nil {
		return nil, nil, fmt.Errorf("build transaction: %w", err)
	}
	// Build client datum
	clientDatum := PlutusData.PlutusData{
		PlutusDataType: PlutusData.PlutusBytes,
		TagNr:          0,
		Value: cbor.NewConstructor(
			1,
			cbor.IndefLengthList{
				clientAddr.PaymentPart,
				[]byte(region),
				curSlotTime.
					Add(time.Duration(duration) * time.Millisecond).
					UnixMilli(),
			},
		),
	}
	// Build mint redeemer
	mintRedeemer := Redeemer.Redeemer{
		Tag: Redeemer.MINT,
		// NOTE: these values are estimated
		ExUnits: Redeemer.ExecutionUnits{
			Mem:   300_000,
			Steps: 100_000_000,
		},
		Data: PlutusData.PlutusData{
			PlutusDataType: PlutusData.PlutusBytes,
			TagNr:          0,
			Value: cbor.NewConstructor(
				0,
				cbor.IndefLengthList{
					clientAddr.PaymentPart,
					[]byte(region),
					selectionId,
					cbor.NewConstructor(
						0,
						cbor.IndefLengthList{
							inputUtxos[0].Input.TransactionId,
							inputUtxos[0].Input.Index,
						},
					),
				},
			),
		},
	}
	apollob, err = apollob.
		// Load all available UTxOs from user's wallet
		AddLoadedUTxOs(availableUtxos...).
		// Explicitly set our chosen inputs
		AddInput(inputUtxos...).
		// Pad out the fee until we figure out why Apollo isn't calculating it correctly
		SetFeePadding(100_000).
		// Set transaction not valid before current slot
		SetValidityStart(int64(curSlot)).
		// Send service payment to provider address
		PayToAddress(
			providerAddress, price,
		).
		// Send client asset to contract
		PayToContract(
			scriptAddress,
			&clientDatum,
			0,
			true,
			apollo.NewUnit(
				hex.EncodeToString(scriptHash),
				string(clientId),
				1,
			),
		).
		// Reference data
		AddReferenceInputV3(
			hex.EncodeToString(refData.TxId),
			int(refData.OutputIdx),
		).
		// Script ref
		AddReferenceInputV3(
			scriptRef.Id().String(),
			int(scriptRef.Index()),
		).
		MintAssetsWithRedeemer(
			apollo.NewUnit(
				hex.EncodeToString(scriptHash),
				string(clientId),
				1,
			),
			mintRedeemer,
		).
		AddRequiredSigner(
			serialization.PubKeyHash(clientAddr.PaymentPart),
		).
		Complete()
	if err != nil {
		return nil, nil, fmt.Errorf("build transaction: %w", err)
	}
	tx := apollob.GetTx()
	cborData, err := cbor.Encode(tx)
	if err != nil {
		return nil, nil, fmt.Errorf("generate transaction CBOR: %w", err)
	}
	return cborData, clientId, nil
}
