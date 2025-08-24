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
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/blinklabs-io/adder/event"
	filter_chainsync "github.com/blinklabs-io/adder/filter/chainsync"
	filter_event "github.com/blinklabs-io/adder/filter/event"
	input_chainsync "github.com/blinklabs-io/adder/input/chainsync"
	output_embedded "github.com/blinklabs-io/adder/output/embedded"
	"github.com/blinklabs-io/adder/pipeline"
	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/blinklabs-io/vpn-indexer/internal/ca"
	"github.com/blinklabs-io/vpn-indexer/internal/client"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	syncStatusLogInterval = 30 * time.Second
)

var (
	metricSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "indexer_slot",
		Help: "Indexer current slot number",
	})
	metricTipSlot = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "indexer_tip_slot",
		Help: "Slot number for upstream chain tip",
	})
)

type Indexer struct {
	cfg               *config.Config
	db                *database.Database
	ca                *ca.Ca
	logger            *slog.Logger
	pipeline          *pipeline.Pipeline
	refTokenPolicyId  lcommon.Blake2b224
	refTokenAssetName []byte
	scriptHash        lcommon.Blake2b224
	tipReached        bool
	syncLogTimer      *time.Timer
	syncStatus        input_chainsync.ChainSyncStatus
}

// Singleton indexer instance
var globalIndexer = &Indexer{}

func (i *Indexer) Start(
	cfg *config.Config,
	logger *slog.Logger,
	db *database.Database,
	ca *ca.Ca,
) error {
	i.cfg = cfg
	i.db = db
	i.ca = ca
	i.logger = logger
	// Parse script address to determine client asset policy ID
	scriptAddr, err := lcommon.NewAddress(cfg.Indexer.ScriptAddress)
	if err != nil {
		return fmt.Errorf("decode script address: %w", err)
	}
	i.scriptHash = scriptAddr.PaymentKeyHash()
	// Parse reference token to determine policy ID and asset name
	refTokenParts := strings.SplitN(cfg.Indexer.ReferenceToken, `.`, 2)
	refTokenPolicyId, err := hex.DecodeString(refTokenParts[0])
	if err != nil {
		return fmt.Errorf("decode reference token policy ID hex: %w", err)
	}
	i.refTokenPolicyId = lcommon.Blake2b224(refTokenPolicyId)
	i.refTokenAssetName, err = hex.DecodeString(refTokenParts[1])
	if err != nil {
		return fmt.Errorf("decode reference token asset name hex: %w", err)
	}
	// Create pipeline
	i.pipeline = pipeline.New()
	// Configure pipeline input
	inputOpts := []input_chainsync.ChainSyncOptionFunc{
		input_chainsync.WithStatusUpdateFunc(i.updateStatus),
		// TODO: re-enable this after https://github.com/blinklabs-io/adder/issues/412 is fixed
		// input_chainsync.WithBulkMode(true),
		input_chainsync.WithAutoReconnect(true),
		input_chainsync.WithLogger(logger),
		input_chainsync.WithDelayConfirmations(cfg.Indexer.DelayConfirmations),
	}
	if cfg.Indexer.NetworkMagic > 0 {
		inputOpts = append(
			inputOpts,
			input_chainsync.WithNetworkMagic(cfg.Indexer.NetworkMagic),
		)
	} else {
		inputOpts = append(
			inputOpts,
			input_chainsync.WithNetwork(cfg.Indexer.Network),
		)
	}
	if cfg.Indexer.Address != "" {
		inputOpts = append(
			inputOpts,
			input_chainsync.WithAddress(cfg.Indexer.Address),
		)
	} else if cfg.Indexer.SocketPath != "" {
		inputOpts = append(
			inputOpts,
			input_chainsync.WithSocketPath(cfg.Indexer.SocketPath),
		)
	}
	cursorPoints, err := i.db.GetCursorPoints()
	if err != nil {
		return err
	}
	if len(cursorPoints) > 0 {
		slog.Info(
			fmt.Sprintf(
				"found previous chainsync cursor(s), latest is: %d, %x",
				cursorPoints[0].Slot,
				cursorPoints[0].Hash,
			),
		)
		inputOpts = append(
			inputOpts,
			input_chainsync.WithIntersectPoints(
				cursorPoints,
			),
		)
	} else if cfg.Indexer.IntersectHash != "" && cfg.Indexer.IntersectSlot > 0 {
		slog.Info(
			fmt.Sprintf("starting new chainsync at configured location: %d, %s", cfg.Indexer.IntersectSlot, cfg.Indexer.IntersectHash),
		)
		hashBytes, err := hex.DecodeString(cfg.Indexer.IntersectHash)
		if err != nil {
			return err
		}
		inputOpts = append(
			inputOpts,
			input_chainsync.WithIntersectPoints(
				[]ocommon.Point{
					{
						Hash: hashBytes,
						Slot: cfg.Indexer.IntersectSlot,
					},
				},
			),
		)
	}
	input := input_chainsync.New(
		inputOpts...,
	)
	i.pipeline.AddInput(input)
	// Configure pipeline filters
	// We only care about transaction events
	filterEvent := filter_event.New(
		filter_event.WithTypes([]string{"chainsync.transaction"}),
	)
	i.pipeline.AddFilter(filterEvent)
	// We only care about transactions involving our script address
	filterChainsync := filter_chainsync.New(
		filter_chainsync.WithAddresses([]string{cfg.Indexer.ScriptAddress}),
	)
	i.pipeline.AddFilter(filterChainsync)
	// Configure pipeline output
	output := output_embedded.New(
		output_embedded.WithCallbackFunc(i.handleEvent),
	)
	i.pipeline.AddOutput(output)
	// Start pipeline
	if err := i.pipeline.Start(); err != nil {
		slog.Error(
			fmt.Sprintf("failed to start pipeline: %s\n", err),
		)
		os.Exit(1)
	}
	// Start error handler
	go func() {
		err, ok := <-i.pipeline.ErrorChan()
		if ok {
			slog.Error(
				fmt.Sprintf("pipeline failed: %s\n", err),
			)
			os.Exit(1)
		}
	}()
	// Schedule periodic catch-up sync log messages
	i.scheduleSyncStatusLog()
	return nil
}

func (i *Indexer) updateStatus(status input_chainsync.ChainSyncStatus) {
	// Store sync status
	i.syncStatus = status
	// Update metrics
	metricSlot.Set(float64(status.SlotNumber))
	metricTipSlot.Set(float64(status.TipSlotNumber))
	// Update chainsync cursor
	blockHash, _ := hex.DecodeString(status.BlockHash)
	cursorPoint := ocommon.Point{
		Hash: blockHash,
		Slot: status.SlotNumber,
	}
	if err := i.db.AddCursorPoint(cursorPoint); err != nil {
		i.logger.Error("failed to update chain cursor", "error", err)
		return
	}
	// Check if we've reached chain tip
	if !i.tipReached && status.TipReached {
		if i.syncLogTimer != nil {
			i.syncLogTimer.Stop()
		}
		i.tipReached = true
		i.logger.Info("caught up to chain tip")
	}
}

func (i *Indexer) handleEvent(evt event.Event) error {
	cfg := config.GetConfig()
	switch evtData := evt.Payload.(type) {
	case input_chainsync.TransactionEvent:
		for _, txOutput := range evtData.Transaction.Produced() {
			// Ignore outputs that aren't to our script address
			if txOutput.Output.Address().String() != cfg.Indexer.ScriptAddress {
				continue
			}
			tmpAssets := txOutput.Output.Assets()
			// Ignore outputs that don't contain assets
			if tmpAssets == nil {
				continue
			}
			// Check for reference token
			if assets := tmpAssets.Assets(i.refTokenPolicyId); len(assets) > 0 {
				if err := i.handleEventReference(txOutput); err != nil {
					return err
				}
			}
			// Check for assets with policy from script hash
			if assets := tmpAssets.Assets(i.scriptHash); len(assets) > 0 {
				if err := i.handleEventClient(txOutput); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("unexpected event type: %T", evt.Payload)
	}
	return nil
}

func (i *Indexer) handleEventClient(txOutput lcommon.Utxo) error {
	// Decode datum
	datum := txOutput.Output.Datum()
	if datum == nil {
		i.logger.Warn(
			fmt.Sprintf(
				"ignoring missing datum in %s",
				txOutput.Id.String(),
			),
		)
		return nil
	}
	var clientDatum ClientDatum
	if _, err := cbor.Decode(datum.Cbor(), &clientDatum); err != nil {
		i.logger.Warn(
			fmt.Sprintf(
				"ignoring unknown client datum format in %s",
				txOutput.Id.String(),
			),
		)
		return nil
	}
	// Determine attached asset name
	var assetName []byte
	if tmpAssets := txOutput.Output.Assets(); tmpAssets != nil {
		if assets := tmpAssets.Assets(i.scriptHash); len(assets) > 0 {
			assetName = assets[0]
		}
	}
	if len(assetName) == 0 {
		i.logger.Warn(
			fmt.Sprintf(
				"ignoring datum without expected asset in %s",
				txOutput.Id.String(),
			),
		)
		return nil
	}
	// Record client datum in database
	err := i.db.AddClient(
		assetName,
		time.Unix(int64(clientDatum.Expiration/1000), 0),
		clientDatum.Credential,
		string(clientDatum.Region),
	)
	if err != nil {
		return err
	}
	// Check region match
	if i.cfg.Vpn.Region != string(clientDatum.Region) {
		// Region doesn't match, return without error
		return nil
	}
	// Generate client
	tmpClient := client.New(i.cfg, i.ca, assetName)
	vpnHost := fmt.Sprintf(
		"%s.%s",
		string(clientDatum.Region),
		i.cfg.Vpn.Domain,
	)
	clientId, err := tmpClient.Generate(vpnHost, i.cfg.Vpn.Port)
	if err != nil {
		return err
	}
	i.logger.Info(
		"generated client",
		"client",
		clientId,
		"tx_output",
		txOutput.Id.String(),
	)
	return nil
}

func (i *Indexer) handleEventReference(txOutput lcommon.Utxo) error {
	// Decode datum
	datum := txOutput.Output.Datum()
	if datum == nil {
		i.logger.Warn(
			fmt.Sprintf(
				"ignoring missing datum in %s",
				txOutput.Id.String(),
			),
		)
		return nil
	}
	var referenceDatum ReferenceDatum
	if _, err := cbor.Decode(datum.Cbor(), &referenceDatum); err != nil {
		i.logger.Warn(
			fmt.Sprintf(
				"ignoring unknown reference datum format in %s",
				txOutput.Id.String(),
			),
		)
		return nil
	}
	// Update in database
	tmpPrices := make([]database.ReferencePrice, 0, len(referenceDatum.Prices))
	for _, price := range referenceDatum.Prices {
		tmpPrices = append(
			tmpPrices,
			database.ReferencePrice{
				Duration: price.Duration,
				Price:    price.Price,
			},
		)
	}
	tmpRegions := make([]string, 0, len(referenceDatum.Regions))
	for _, region := range referenceDatum.Regions {
		tmpRegions = append(tmpRegions, string(region))
	}
	if err := i.db.UpdateReferenceData(txOutput.Id, tmpPrices, tmpRegions); err != nil {
		return fmt.Errorf("update reference in database: %w", err)
	}
	i.logger.Info(
		"updated reference data",
		"tx_output",
		txOutput.Id.String(),
	)
	return nil
}

func (i *Indexer) scheduleSyncStatusLog() {
	i.syncLogTimer = time.AfterFunc(syncStatusLogInterval, i.syncStatusLog)
}

func (i *Indexer) syncStatusLog() {
	slog.Info(
		fmt.Sprintf(
			"catch-up sync in progress: at %d.%s (current tip slot is %d)",
			i.syncStatus.SlotNumber,
			i.syncStatus.BlockHash,
			i.syncStatus.TipSlotNumber,
		),
	)
	i.scheduleSyncStatusLog()
}

// GetIndexer returns the global indexer instance
func GetIndexer() *Indexer {
	return globalIndexer
}
