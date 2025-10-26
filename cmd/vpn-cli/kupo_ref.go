package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/SundaeSwap-finance/kugo"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"github.com/blinklabs-io/vpn-indexer/internal/database"
)

func getKupoClient() (*kugo.Client, error) {
	cfg := config.GetConfig()
	if cfg.TxBuilder.KupoUrl == "" {
		return nil, errors.New("no kupo url provided")
	}
	k := kugo.New(kugo.WithEndpoint(cfg.TxBuilder.KupoUrl))
	if k == nil {
		return nil, fmt.Errorf("failed kupo client: %s", cfg.TxBuilder.KupoUrl)
	}
	return k, nil
}

func loadReferenceFromKugoClient(
	ctx context.Context,
) (database.Reference, error) {
	cfg := config.GetConfig()
	if strings.TrimSpace(cfg.TxBuilder.KupoUrl) == "" {
		return database.Reference{}, errors.New("kupo url not configured")
	}
	k, err := getKupoClient()
	if err != nil {
		return database.Reference{}, err
	}

	if tok := strings.TrimSpace(cfg.Indexer.ReferenceToken); tok != "" {
		if ref, err := refByKugoMatches(ctx, k, tok); err == nil {
			return ref, nil
		}
	}
	if addr := strings.TrimSpace(cfg.Indexer.ScriptAddress); addr != "" {
		return refByKugoMatches(ctx, k, addr)
	}

	return database.Reference{}, errors.New(
		"neither reference token nor script address configured",
	)
}

func refByKugoMatches(
	ctx context.Context,
	k *kugo.Client,
	pattern string,
) (database.Reference, error) {
	// Query matches by pattern (address).
	matches, err := k.Matches(ctx, kugo.Pattern(pattern))
	if err != nil {
		return database.Reference{}, fmt.Errorf("kupo matches: %w", err)
	}
	if len(matches) == 0 {
		return database.Reference{}, fmt.Errorf(
			"no matches for pattern %q",
			pattern,
		)
	}
	match := matches[0]

	// Decode transaction_id (bytes to hex)
	txid, err := hex.DecodeString(parseHex(match.TransactionID))
	if err != nil {
		return database.Reference{}, fmt.Errorf("txid decode: %w", err)
	}

	// Acquire datum CBOR using kugo only
	var datumCBOR []byte
	switch {
	case strings.TrimSpace(getDatumHash(match)) != "":
		hexStr, derr := k.Datum(ctx, parseHex(getDatumHash(match)))
		if derr != nil {
			return database.Reference{}, fmt.Errorf(
				"fetch datum by hash: %w",
				derr,
			)
		}
		datumCBOR, err = hex.DecodeString(parseHex(hexStr))
		if err != nil {
			return database.Reference{}, fmt.Errorf("datum decode: %w", err)
		}
	default:
		return database.Reference{}, errors.New(
			"reference utxo has no datum (no bytes/hash)",
		)
	}

	// Decode reference plans and regions from datumCBOR
	plans, regions, err := decodeRefDatumFlexible(datumCBOR)
	if err != nil {
		return database.Reference{}, fmt.Errorf("decode ref datum: %w", err)
	}

	// Map to database.Reference expected by BuildSignupTx
	refPrices := make([]database.ReferencePrice, 0, len(plans))
	for _, p := range plans {
		refPrices = append(refPrices, database.ReferencePrice{
			Duration: p.Duration,
			Price:    p.Price,
		})
	}
	refRegions := make([]database.ReferenceRegion, 0, len(regions))
	for _, r := range regions {
		refRegions = append(refRegions, database.ReferenceRegion{Name: r})
	}

	return database.Reference{
		TxId:      txid,
		OutputIdx: match.OutputIndex,
		Prices:    refPrices,
		Regions:   refRegions,
	}, nil
}

func getDatumHash(m kugo.Match) string {
	return m.DatumHash
}

func parseHex(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "0X")
	return s
}
