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
	"bytes"
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

type RenewDeps struct {
	DB     *database.Database
	Ref    *database.Reference
	Client *database.Client
}

func BuildRenewTransferTx(
	deps RenewDeps,
	paymentAddress string,
	ownerAddress string,
	clientId string,
	price int,
	duration int,
) ([]byte, error) {
	// Validate inputs
	if paymentAddress == "" {
		return nil, NewInputValidationError("empty payment address provided")
	}
	cfg := config.GetConfig()
	cc, err := apolloBackend()
	if err != nil {
		return nil, err
	}
	// Lookup current client information
	clientAssetName, err := hex.DecodeString(clientId)
	if err != nil {
		return nil, fmt.Errorf("decode client ID: %w", err)
	}
	var client database.Client
	switch {
	case deps.Client != nil:
		client = *deps.Client
	case deps.DB != nil:
		client, err = deps.DB.ClientByAssetName(clientAssetName)
		if err != nil {
			return nil, fmt.Errorf("lookup client (db): %w", err)
		}
	default:
		return nil, errors.New("renew: deps.Client not provided and no fallback (DB) available")
	}
	// Decode payment address
	paymentAddr, err := serAddress.DecodeAddress(paymentAddress)
	if err != nil {
		return nil, NewInputValidationError("failed to decode payment address")
	}
	// Determine owner credential
	// Use existing owner for client by default
	ownerCredential := client.Credential
	if ownerAddress != "" && ownerAddress != paymentAddress {
		ownerAddr, err := serAddress.DecodeAddress(ownerAddress)
		if err != nil {
			return nil, NewInputValidationError("failed to decode owner address")
		}
		ownerCredential = ownerAddr.PaymentPart
	}
	// Determine if the owner is changing
	newOwnerCred := []byte{}
	if !bytes.Equal(ownerCredential, client.Credential) {
		newOwnerCred = ownerCredential
	}
	// Decode script address
	scriptAddress, err := serAddress.DecodeAddress(cfg.Indexer.ScriptAddress)
	if err != nil {
		return nil, fmt.Errorf("script address: %w", err)
	}
	scriptHash := scriptAddress.PaymentPart
	// Decode provider address
	providerAddress, err := serAddress.DecodeAddress(
		cfg.TxBuilder.ProviderAddress,
	)
	if err != nil {
		return nil, fmt.Errorf("provider address: %w", err)
	}

	// Resolve reference data: prefer deps.Ref, fall back to DB
	var refData database.Reference
	switch {
	case deps.Ref != nil:
		refData = *deps.Ref
	case deps.DB != nil:
		refData, err = deps.DB.ReferenceData()
		if err != nil {
			return nil, fmt.Errorf("reference data: %w", err)
		}
	default:
		refData, err = deps.DB.ReferenceData()
		if err != nil {
			return nil, fmt.Errorf("reference data: %w", err)
		}
	}
	// Parse script ref
	scriptRef, err := inputRefFromString(cfg.TxBuilder.ScriptRefInput)
	if err != nil {
		return nil, err
	}
	// Get available UTxOs from user's wallet
	availableUtxos, err := cc.Utxos(paymentAddr)
	if err != nil {
		return nil, fmt.Errorf(
			"lookup UTxOs for address: %s: %w",
			paymentAddr.String(),
			err,
		)
	}
	// Choose input UTxOs from user's wallet
	inputUtxos, err := chooseInputUtxos(availableUtxos, price+5_000_000)
	if err != nil {
		return nil, fmt.Errorf("choose input UTxOs: %w", err)
	}
	if len(inputUtxos) == 0 {
		return nil, NewInputValidationError("no input UTxOs found")
	}
	// Lookup UTxO for client asset
	clientUtxo, err := cc.GetUtxoFromRef(
		hex.EncodeToString(client.TxHash),
		int(client.TxOutputIndex), // nolint:gosec
	)
	if err != nil {
		return nil, fmt.Errorf("lookup client UTxO: %w", err)
	}
	// Determine plan selection
	// The default of -1 represents transfer without renewal
	selectionId := -1
	// Lookup plan by price/duration, if provided
	if price > 0 && duration > 0 {
		selectionId, err = determinePlanSelection(refData, price, duration)
		if err != nil {
			return nil, NewInputValidationError("could not determine plan selection from provided price/duration")
		}
	}
	// Get last known slot
	curSlot, err := cc.LastBlockSlot()
	if err != nil {
		return nil, fmt.Errorf("query latest block slot: %w", err)
	}
	// Determine new expiration
	var newExpiry time.Time
	if time.Now().After(client.Expiration) {
		// Previous client has expired, so we calculate expiration from the last known slot
		ogmios := OgmiosClient()
		systemStart, err := ogmiosSystemStart(ogmios)
		if err != nil {
			return nil, fmt.Errorf("query system start: %w", err)
		}
		eraHistory, err := ogmios.EraSummaries(context.Background())
		if err != nil {
			return nil, fmt.Errorf("query era summaries: %w", err)
		}
		curSlotTime := systemStart.Add(
			time.Duration(
				ogmigo.SlotToElapsedMilliseconds(
					eraHistory,
					uint64(curSlot),
				),
			) * time.Millisecond,
		)
		newExpiry = curSlotTime.
			Add(time.Duration(duration) * time.Millisecond)
	} else {
		// Existing client is not expired, so we add the new duration to the end
		newExpiry = client.Expiration.
			Add(time.Duration(duration) * time.Millisecond)
	}
	// Configure transaction builder
	apollob := apollo.New(cc)
	apollob, err = apollob.
		SetWalletFromBech32(paymentAddress).
		SetWalletAsChangeAddress()
	if err != nil {
		return nil, fmt.Errorf("build transaction: %w", err)
	}
	// Build client datum
	clientDatum := PlutusData.PlutusData{
		PlutusDataType: PlutusData.PlutusBytes,
		TagNr:          0,
		Value: cbor.NewConstructor(
			1,
			cbor.IndefLengthList{
				ownerCredential,
				[]byte(client.Region),
				newExpiry.UnixMilli(),
			},
		),
	}
	// Build spend redeemer
	redeemer := Redeemer.Redeemer{
		Tag: Redeemer.SPEND,
		// NOTE: these values are estimated
		ExUnits: Redeemer.ExecutionUnits{
			Mem:   400_000,
			Steps: 110_000_000,
		},
		Data: PlutusData.PlutusData{
			PlutusDataType: PlutusData.PlutusBytes,
			TagNr:          0,
			Value: cbor.NewConstructor(
				2,
				cbor.IndefLengthList{
					newOwnerCred,
					clientAssetName,
					selectionId,
				},
			),
		},
	}
	apollob = apollob.
		// Load all available UTxOs from user's wallet
		AddLoadedUTxOs(availableUtxos...).
		// Explicitly set our chosen inputs
		AddInput(inputUtxos...).
		// Pad out the fee until we figure out why Apollo isn't calculating it correctly
		SetFeePadding(200_000).
		// Set transaction not valid before current slot
		SetValidityStart(int64(curSlot)).
		// Set TTL
		SetTtl(int64(curSlot+transactionTtlSlots)).
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
				string(clientAssetName),
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
		CollectFrom(
			*clientUtxo,
			redeemer,
		)
	// We only require the current owner to sign if we're changing ownership
	if len(newOwnerCred) > 0 {
		apollob = apollob.AddRequiredSigner(
			serialization.PubKeyHash(client.Credential),
		)
	}
	apollob, err = apollob.Complete()
	if err != nil {
		return nil, fmt.Errorf("build transaction: %w", err)
	}
	tx := apollob.GetTx()
	cborData, err := cbor.Encode(tx)
	if err != nil {
		return nil, fmt.Errorf("generate transaction CBOR: %w", err)
	}
	return cborData, nil
}
