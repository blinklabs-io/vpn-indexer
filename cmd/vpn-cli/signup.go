package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/blinklabs-io/vpn-indexer/internal/txbuilder"
	"github.com/spf13/cobra"
)

var (
	outPath         string
	format          string
	flagPaymentAddr string
	flagOwnerAddr   string
	flagPrice       int
	flagDuration    int
	flagRegion      string

	flagOgmiosURL string
	flagKupoURL   string
)

var rootCmd = &cobra.Command{
	Use:   "vpn-cli",
	Short: "Build unsigned transactions for VPN",
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&outPath, "out", "o", "", "output file (default: stdout)")
	rootCmd.PersistentFlags().StringVar(&format, "format", "hex", "output format: hex|cbor")

	cmd := &cobra.Command{
		Use:   "signup",
		Short: "Build an unsigned signup transaction",
		RunE:  runSignup,
	}
	cmd.Flags().StringVar(&flagPaymentAddr, "payment", "", "client payment bech32 address")
	cmd.Flags().StringVar(&flagOwnerAddr, "owner", "", "owner bech32 address")
	cmd.Flags().IntVar(&flagPrice, "price", 0, "plan price in lovelace")
	cmd.Flags().IntVar(&flagDuration, "duration", 0, "plan duration in milliseconds")
	cmd.Flags().StringVar(&flagRegion, "region", "", "region code")

	// Load from on chain using Kupo/Ogmios
	cmd.Flags().StringVar(&flagOgmiosURL, "ogmios-url", "", "Ogmios endpoint (optional)")
	cmd.Flags().StringVar(&flagKupoURL, "kupo-url", "", "Kupo endpoint (used if --refdata not provided)")

	_ = cmd.MarkFlagRequired("payment")
	_ = cmd.MarkFlagRequired("price")
	_ = cmd.MarkFlagRequired("duration")
	_ = cmd.MarkFlagRequired("region")

	rootCmd.AddCommand(cmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSignup(cmd *cobra.Command, _ []string) error {
	if err := validateFormat(format); err != nil {
		return err
	}

	if flagPaymentAddr == "" {
		return errors.New("--client is required")
	}
	if flagPrice <= 0 {
		return errors.New("--price must be > 0")
	}
	if flagDuration <= 0 {
		return errors.New("--duration must be > 0")
	}
	if flagRegion == "" {
		return errors.New("--region is required")
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

	// Build the unsigned transaction using same path that API used
	cborBytes, _, err := txbuilder.BuildSignupTx(
		txbuilder.SignupDeps{Ref: &ref},
		flagPaymentAddr,
		flagOwnerAddr,
		flagPrice,
		flagDuration,
		flagRegion,
	)
	if err != nil {
		return err
	}

	return writeOutHelper(format, outPath, cborBytes)
}
