package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/blinklabs-io/vpn-indexer/internal/database"
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

	flagRefJSON    string
	flagOgmiosURL  string
	flagKupoURL    string
	flagScriptAddr string
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
	cmd.Flags().IntVar(&flagDuration, "duration", 0, "plan duration (contract unit)")
	cmd.Flags().StringVar(&flagRegion, "region", "", "region code")

	// Load from on chain using Kupo/Ogmios
	cmd.Flags().StringVar(&flagRefJSON, "refdata", "", "path to reference data JSON (optional)")
	cmd.Flags().StringVar(&flagOgmiosURL, "ogmios-url", "", "Ogmios endpoint (optional)")
	cmd.Flags().StringVar(&flagKupoURL, "kupo-url", "", "Kupo endpoint (used if --refdata not provided)")
	cmd.Flags().StringVar(&flagScriptAddr, "script-address", "", "script address holding reference UTxO (used if --refdata not provided)")

	_ = cmd.MarkFlagRequired("client")
	_ = cmd.MarkFlagRequired("price")
	_ = cmd.MarkFlagRequired("duration")
	_ = cmd.MarkFlagRequired("region")
	_ = cmd.MarkFlagRequired("refdata") // refadata will be required till we load from on chain

	rootCmd.AddCommand(cmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSignup(cmd *cobra.Command, _ []string) error {
	switch strings.ToLower(format) {
	case "hex", "cbor":
	default:
		return fmt.Errorf("invalid --format %q (must be hex|cbor)", format)
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

	txbuilder.SetChainEndpoints(flagOgmiosURL, flagKupoURL)

	var ref database.Reference
	var err error

	if strings.TrimSpace(flagRefJSON) != "" {
		f, ferr := os.Open(flagRefJSON)
		if ferr != nil {
			return fmt.Errorf("open refdata: %w", ferr)
		}
		defer func() {
			_ = f.Close()
		}()
		ref, err = database.ReferenceFromJSON(f)
	} else {
		if strings.TrimSpace(flagKupoURL) == "" || strings.TrimSpace(flagScriptAddr) == "" {
			return errors.New("missing chain flags: provide --kupo-url and --script-address, or use --refdata")
		}
		// TODO: Need to write the code to load the ref data from chain using flagKupoURL, flagOgmiosURL
		ref, err = database.LoadFromChain(context.Background(), flagOgmiosURL, flagKupoURL, flagScriptAddr)
	}
	if err != nil {
		return err
	}

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

	switch strings.ToLower(format) {
	case "hex":
		s := strings.ToUpper(hex.EncodeToString(cborBytes)) + "\n"
		return writeOut(outPath, []byte(s))
	case "cbor":
		return writeOut(outPath, cborBytes)
	}
	return nil
}

func writeOut(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
