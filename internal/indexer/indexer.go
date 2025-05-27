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
	"time"

	"github.com/blinklabs-io/adder/event"
	filter_chainsync "github.com/blinklabs-io/adder/filter/chainsync"
	filter_event "github.com/blinklabs-io/adder/filter/event"
	input_chainsync "github.com/blinklabs-io/adder/input/chainsync"
	output_embedded "github.com/blinklabs-io/adder/output/embedded"
	"github.com/blinklabs-io/adder/pipeline"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
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
	db           *database.Database
	pipeline     *pipeline.Pipeline
	tipReached   bool
	syncLogTimer *time.Timer
	syncStatus   input_chainsync.ChainSyncStatus
}

// Singleton indexer instance
var globalIndexer = &Indexer{}

func (i *Indexer) Start(cfg *config.Config, logger *slog.Logger, db *database.Database) error {
	i.db = db
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
	/*
		cursorSlotNumber, cursorBlockHash, err := state.GetState().GetCursor()
		if err != nil {
			return err
		}
	*/
	// TODO: remove these defaults in favor of getting intersect point(s) from state
	var cursorSlotNumber uint64 = 0
	var cursorBlockHash string
	if cursorSlotNumber > 0 {
		slog.Info(
			fmt.Sprintf(
				"found previous chainsync cursor: %d, %s",
				cursorSlotNumber,
				cursorBlockHash,
			),
		)
		hashBytes, err := hex.DecodeString(cursorBlockHash)
		if err != nil {
			return err
		}
		inputOpts = append(
			inputOpts,
			input_chainsync.WithIntersectPoints(
				[]ocommon.Point{
					{
						Hash: hashBytes,
						Slot: cursorSlotNumber,
					},
				},
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
	i.syncStatus = status
	metricSlot.Set(float64(status.SlotNumber))
	metricTipSlot.Set(float64(status.TipSlotNumber))
	/*
		if err := state.GetState().UpdateCursor(status.SlotNumber, status.BlockHash); err != nil {
			slog.Error(
				fmt.Sprintf("failed to update cursor: %s", err),
			)
		}
	*/
	if !i.tipReached && status.TipReached {
		if i.syncLogTimer != nil {
			i.syncLogTimer.Stop()
		}
		i.tipReached = true
		slog.Info("caught up to chain tip")
	}
}

func (i *Indexer) handleEvent(evt event.Event) error {
	// TODO
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
