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
	"crypto/rand"
	"fmt"
	"time"

	"github.com/Salvionied/apollo/txBuilding/Backend/OgmiosChainContext"
	"github.com/SundaeSwap-finance/kugo"
	"github.com/SundaeSwap-finance/ogmigo/v6"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
)

const (
	defaultKupoTimeout = 1 * time.Second
)

func apolloBackend() (*OgmiosChainContext.OgmiosChainContext, error) {
	cfg := config.GetConfig()
	ogmiosClient := ogmigo.New(
		ogmigo.WithEndpoint(cfg.TxBuilder.OgmiosUrl),
	)
	kupoClient := kugo.New(
		kugo.WithEndpoint(cfg.TxBuilder.KupoUrl),
		kugo.WithTimeout(defaultKupoTimeout),
	)
	occ := OgmiosChainContext.NewOgmiosChainContext(*ogmiosClient, *kupoClient)
	return &occ, nil
}

func inputRefFromString(ref string) (lcommon.TransactionInput, error) {
	var refInput shelley.ShelleyTransactionInput
	err := fmt.Sscanf(
		cfg.TxBuilder.ScriptRefInput,
		"%x#%d",
		&refInput.TxId,
		&refInput.OutputIdx,
	)
	if err != nil {
		return nil, fmt.Errorf("parse script ref input: %w", err)
	}
	return refInput, nil
}

func randomClientId() ([]byte, error) {
	tmp := make([]byte, 0, 32)
	if err := rand.Read(tmp); err != nil {
		return nil, err
	}
	hash := lcommon.Blake2b256Hash(tmp)
	return hash, nil
}
