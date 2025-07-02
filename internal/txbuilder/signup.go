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
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Salvionied/apollo"
	"github.com/Salvionied/apollo/serialization"
	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
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
) ([]byte, error) {
	cfg := config.GetConfig()
	cc, err := apolloBackend()
	if err != nil {
		return nil, err
	}
	clientAddr, err := serAddress.DecodeAddress(clientAddress)
	if err != nil {
		return nil, fmt.Errorf("client address: %w", err)
	}
	scriptAddress, err := serAddress.DecodeAddress(cfg.Indexer.ScriptAddress)
	if err != nil {
		return nil, fmt.Errorf("script address: %w", err)
	}
	scriptHash := scriptAddress.PaymentPart
	providerAddress, err := serAddress.DecodeAddress(cfg.TxBuilder.ProviderAddress)
	if err != nil {
		return nil, fmt.Errorf("provider address: %w", err)
	}
	// Lookup reference data
	refData, err := db.ReferenceData()
	if err != nil {
		return nil, fmt.Errorf("reference data: %w", err)
	}
	// Parse script ref
	scriptRef, err := inputRefFromString(cfg.TxBuilder.ScriptRefInput)
	if err != nil {
		return nil, err
	}
	/*
		// TODO: remove me
		utxo, err := cc.GetUtxoFromRef(scriptRef.Id().String(), int(scriptRef.Index()))
		if err != nil {
			fmt.Printf("err = %v\n", err)
		}
		fmt.Printf("utxo = %#v\n", utxo)
	*/
	// Get available UTxOs from user's wallet
	availableUtxos, err := cc.Utxos(clientAddr)
	if err != nil {
		return nil, fmt.Errorf("lookup UTxOs for address: %s: %w", clientAddr.String(), err)
	}
	// Choose input UTxOs from user's wallet
	inputUtxos, err := chooseInputUtxos(availableUtxos, price+5_000_000)
	if err != nil {
		return nil, fmt.Errorf("choose input UTxOs: %w", err)
	}
	if len(inputUtxos) == 0 {
		return nil, errors.New("no input UTxOs found")
	}
	// Determine client ID from first selected input UTxO
	clientId := clientIdFromInput(inputUtxos[0].Input)
	fmt.Printf("clientId(%d) = %x\n", len(clientId), clientId)
	// Determine plan selection ID from price/duration
	selectionId, err := determinePlanSelection(refData, price, duration)
	if err != nil {
		return nil, fmt.Errorf("determine plan selection: %w", err)
	}
	apollob := apollo.New(cc)
	apollob, err = apollob.
		SetWalletFromBech32(clientAddress).
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
				clientAddr.PaymentPart,
				[]byte(region),
				time.Now().Add(time.Duration(duration) * time.Millisecond).UnixMilli(),
			},
		),
	}
	// Build mint redeemer
	mintRedeemer := Redeemer.Redeemer{
		Tag: Redeemer.MINT,
		/*
			// NOTE: these values are estimated
			ExUnits: Redeemer.ExecutionUnits{
				Mem:   280_000,
				Steps: 130_000_000,
			},
		*/
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
		AddLoadedUTxOs(availableUtxos...).
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
		// TODO: build TX
		/*
		   --read-only-tx-in-reference $UTXO_VPN_REF_DATA \
		   --mint "1 $VPN_CS.$TN" \
		   --mint-tx-in-reference $VPN_TX_REF \
		   --mint-plutus-script-v3 \
		   --mint-reference-tx-in-redeemer-file $REDEEMERS_PATH/user1_mint.json \
		   --policy-id $VPN_CS \
		   --tx-out $VPN_ADDR+2000000+"1 $VPN_CS.$TN" \
		   --tx-out-inline-datum-file $DATUM_PATH \
		   --tx-out $(cat $WALLET_PATH/provider.addr)+$price \
		   --change-address $USER_ADDR \
		   --invalid-before $cur_slot \
		   --required-signer $WALLET_PATH/$USER.skey
		*/
		Complete()
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
