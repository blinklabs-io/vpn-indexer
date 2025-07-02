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
	"fmt"
	"time"

	"github.com/Salvionied/apollo"
	"github.com/Salvionied/apollo/serialization"
	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/Salvionied/apollo/serialization/PlutusData"
	"github.com/Salvionied/apollo/serialization/Redeemer"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
)

func BuildSignupTx(
	clientAddress string,
	price uint64,
	duration time.Duration,
	region string,
) ([]byte, error) {
	cfg := config.GetConfig()
	cc, err := apolloBackend()
	if err != nil {
		return nil, err
	}
	scriptAddress, _ := serAddress.DecodeAddress(cfg.Indexer.ScriptAddress)
	providerAddress, _ := serAddress.DecodeAddress(cfg.TxBuilder.ProviderAddress)
	// TODO: lookup reference token UTxO ID using apollo backend
	scriptRef, err := inputRefFromString(cfg.TxBuilder.ScriptRefInput)
	if err != nil {
		return nil, err
	}
	clientId, err := randomClientId()
	if err != nil {
		return nil, err
	}
	apollob := apollo.New(cc)
	apollob, err = apollob.
		SetWalletFromBech32(clientAddress).
		SetWalletAsChangeAddress()
	if err != nil {
		return nil, fmt.Errorf("build transaction: %w", err)
	}
	// TODO
	mintRedeemer := Redeemer.Redeemer{
		Tag: Redeemer.MINT,
		// NOTE: these values are estimated
		ExUnits: Redeemer.ExecutionUnits{
			Mem:   280_000,
			Steps: 130_000_000,
		},
		Data: PlutusData.PlutusData{
			PlutusDataType: PlutusData.PlutusBytes,
			TagNr:          0,
			Value: cbor.NewConstructor(
				1,
				cbor.IndefLengthList{
					cbor.NewConstructor(
						0,
						cbor.IndefLengthList{
							cbor.NewConstructor(
								0,
								cbor.IndefLengthList{
									validatorOutRef.Input.TransactionId,
								},
							),
							validatorOutRef.Input.Index,
						},
					),
					blockDataBlockNumber - 1,
				},
			),
		},
	}
	apollob, err = apollob.
		// TODO
		PayToAddress(
			providerAddress, 2000000, // TODO: amount
		).
		PayToContract(
			contractAddress,
			&postDatum,
			int(validatorOutRef.Output.PostAlonzo.Amount.Am.Coin),
			true,
			apollo.NewUnit(mintValidatorHash, "TUNA"+string(validatorHashBytes), 1),
		).
		AddReferenceInput(
			refInput.TxId,
			int(refInput.OutputIdx),
		).
		MintAssetsWithRedeemer(
			apollo.NewUnit(mintValidatorHash, "TUNA", 5000000000),
			mintRedeemer,
		).
		AddRequiredSigner(
			serialization.PubKeyHash(userPkh),
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
	cborData, err := cbor.Marshal(tx)
	if err != nil {
		return nil, fmt.Errorf("generate transaction CBOR: %w", err)
	}
	return cborData, nil
}
