package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blinklabs-io/vpn-indexer/internal/txbuilder"
	"github.com/spf13/cobra"
)

var (
	flagTransferPayment  string
	flagTransferOwner    string
	flagTransferClientID string

	flagTransferOgmiosURL string
	flagTransferKupoURL   string
)

func init() {
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Build an unsigned transfer transaction",
		RunE:  runTransfer,
	}

	cmd.Flags().StringVar(&flagTransferPayment, "payment", "", "client payment bech32 address (required)")
	cmd.Flags().StringVar(&flagTransferOwner, "owner", "", "owner bech32 address (required)")
	cmd.Flags().StringVar(&flagTransferClientID, "client-id", "", "existing client ID (required)")

	// Load from on chain using Kupo/Ogmios
	cmd.Flags().StringVar(&flagTransferOgmiosURL, "ogmios-url", "", "Ogmios endpoint (optional)")
	cmd.Flags().StringVar(&flagTransferKupoURL, "kupo-url", "", "Kupo endpoint (used if --refdata not provided)")

	_ = cmd.MarkFlagRequired("payment")
	_ = cmd.MarkFlagRequired("owner")
	_ = cmd.MarkFlagRequired("client-id")

	rootCmd.AddCommand(cmd)
}

func runTransfer(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(format); err != nil {
		return err
	}
	// Normalize inputs
	flagTransferPayment = strings.TrimSpace(flagTransferPayment)
	flagTransferOwner = strings.TrimSpace(flagTransferOwner)
	flagTransferClientID = strings.TrimSpace(flagTransferClientID)

	if flagTransferPayment == "" {
		return errors.New("--payment is required")
	}
	if flagTransferOwner == "" {
		return errors.New("--owner is required")
	}
	if strings.TrimSpace(flagTransferClientID) == "" {
		return errors.New("--client-id is required")
	}

	cfg, err := initConfig(flagTransferKupoURL, flagTransferOgmiosURL)
	if err != nil {
		return err
	}
	if err := requireEndpoints(cfg); err != nil {
		return err
	}
	ref, err := loadRefData(cmd.Context())
	if err != nil {
		return fmt.Errorf("load reference (kupo): %w", err)
	}

	// find the client
	client, err := findClientOnChain(cmd.Context(), flagTransferClientID)
	if err != nil {
		return fmt.Errorf("find client (kupo): %w", err)
	}

	// Build unsigned tx using txbuilder.
	cborBytes, err := txbuilder.BuildRenewTransferTx(
		txbuilder.RenewDeps{
			Ref:    &ref,
			Client: &client,
		},
		flagTransferPayment,
		flagTransferOwner,
		flagTransferClientID,
		0,
		0,
	)
	if err != nil {
		return err
	}

	return writeOutHelper(format, outPath, cborBytes)
}
