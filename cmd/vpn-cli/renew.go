package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blinklabs-io/vpn-indexer/internal/txbuilder"
	"github.com/spf13/cobra"
)

var (
	flagRenewPayment  string
	flagRenewOwner    string
	flagRenewClientID string
	flagRenewPrice    int
	flagRenewDuration int

	flagRenewOgmiosURL string
	flagRenewKupoURL   string
)

func init() {
	cmd := &cobra.Command{
		Use:   "renew",
		Short: "Build an unsigned renewal transaction",
		RunE:  runRenew,
	}

	cmd.Flags().StringVar(&flagRenewPayment, "payment", "", "client payment bech32 address (required)")
	cmd.Flags().StringVar(&flagRenewOwner, "owner", "", "owner bech32 address")
	cmd.Flags().StringVar(&flagRenewClientID, "client-id", "", "existing client ID (required)")
	cmd.Flags().IntVar(&flagRenewPrice, "price", 0, "plan price in lovelace")
	cmd.Flags().IntVar(&flagRenewDuration, "duration", 0, "plan duration in milliseconds")

	// Load from on chain using Kupo/Ogmios
	cmd.Flags().StringVar(&flagOgmiosURL, "ogmios-url", "", "Ogmios endpoint (optional)")
	cmd.Flags().StringVar(&flagKupoURL, "kupo-url", "", "Kupo endpoint (used if --refdata not provided)")

	_ = cmd.MarkFlagRequired("payment")
	_ = cmd.MarkFlagRequired("price")
	_ = cmd.MarkFlagRequired("duration")
	_ = cmd.MarkFlagRequired("client-id")

	rootCmd.AddCommand(cmd)
}

func runRenew(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(format); err != nil {
		return err
	}
	if flagRenewPayment == "" {
		return errors.New("--payment is required")
	}
	if flagRenewPrice <= 0 {
		return errors.New("--price must be > 0")
	}
	if flagRenewDuration <= 0 {
		return errors.New("--duration must be > 0")
	}
	if strings.TrimSpace(flagRenewClientID) == "" {
		return errors.New("--client-id is required")
	}

	cfg, err := initConfig(flagKupoURL, flagOgmiosURL)
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
	client, err := findClientOnChain(cmd.Context(), flagRenewClientID)
	if err != nil {
		return fmt.Errorf("find client (kupo): %w", err)
	}

	// Build unsigned tx using txbuilder.
	cborBytes, err := txbuilder.BuildRenewTransferTx(
		txbuilder.RenewDeps{
			Ref:    &ref,
			Client: &client,
		},
		flagRenewPayment,
		flagRenewOwner,
		flagRenewClientID,
		flagRenewPrice,
		flagRenewDuration,
	)
	if err != nil {
		return err
	}

	return writeOutHelper(format, outPath, cborBytes)
}
