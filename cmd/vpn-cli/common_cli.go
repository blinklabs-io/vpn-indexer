package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
	"github.com/blinklabs-io/vpn-indexer/internal/txbuilder"
)

// It makes sure --format is hex or cbor
func validateFormat(format string) error {
	switch strings.ToLower(format) {
	case "hex", "cbor":
		return nil
	default:
		return fmt.Errorf("invalid --format %q (must be hex|cbor)", format)
	}
}

// loads global config and applies CLI overrides.
func initConfig(kupoURL, ogmiosURL string) (*config.Config, error) {
	if _, err := config.Load(""); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	cfg := config.GetConfig()
	if kupoURL != "" {
		cfg.TxBuilder.KupoUrl = kupoURL
	}
	if ogmiosURL != "" {
		cfg.TxBuilder.OgmiosUrl = ogmiosURL
		// reset Apollo/Ogmios-derived cache in txbuilder path
		txbuilder.ResetCachedSystemStart()
	}
	return cfg, nil
}

// It makes sure both Kupo and Ogmios URLs exist
func requireEndpoints(cfg *config.Config) error {
	if strings.TrimSpace(cfg.TxBuilder.KupoUrl) == "" {
		return errors.New("kupo url is required (set --kupo-url)")
	}
	if strings.TrimSpace(cfg.TxBuilder.OgmiosUrl) == "" {
		return errors.New("ogmios url is required (set --ogmios-url)")
	}
	return nil
}

func loadRefData(ctx context.Context) (database.Reference, error) {
	return loadReferenceFromKugoClient(ctx)
}

// Ir writes CBOR/HEX based on format
func writeOutHelper(format, outPath string, cborBytes []byte) error {
	switch strings.ToLower(format) {
	case "hex":
		s := strings.ToUpper(hex.EncodeToString(cborBytes)) + "\n"
		return writeOut(outPath, []byte(s))
	case "cbor":
		return writeOut(outPath, cborBytes)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

func writeOut(path string, data []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
