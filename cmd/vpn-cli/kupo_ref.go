package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	serAddress "github.com/Salvionied/apollo/serialization/Address"
	"github.com/SundaeSwap-finance/kugo"
	"github.com/blinklabs-io/gouroboros/cbor"
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

// It finds the client UTXO based on script address & fetches its datum & decodes it.
func findClientOnChain(ctx context.Context, clientIdHex string) (database.Client, error) {
	cfg := config.GetConfig()
	if strings.TrimSpace(cfg.Indexer.ScriptAddress) == "" {
		return database.Client{}, errors.New("script address not configured")
	}

	// script address contains policy id & asset name
	scriptAddrBytes, err := serAddress.DecodeAddress(cfg.Indexer.ScriptAddress)
	if err != nil {
		return database.Client{}, fmt.Errorf("failed in decoding script address: %w", err)
	}
	policyHex := strings.ToLower(hex.EncodeToString(scriptAddrBytes.PaymentPart))
	targetAsset := strings.ToLower(parseHex(clientIdHex))

	k, err := getKupoClient()
	if err != nil {
		return database.Client{}, err
	}

	// Search for all UTXOs that contains the below asset pattern
	assetPattern := fmt.Sprintf("%s.%s", policyHex, targetAsset)
	matches, err := k.Matches(ctx, kugo.Pattern(assetPattern))
	if err != nil {
		return database.Client{}, fmt.Errorf("kupo matches(script address): %w", err)
	}

	if len(matches) == 0 {
		return database.Client{}, fmt.Errorf("client not found for clientId=%s", clientIdHex)
	}

	// From all UTXOs, pick the one at script address
	var picked *kugo.Match
	for i := range matches {
		m := matches[i]
		if strings.EqualFold(strings.TrimSpace(m.Address), strings.TrimSpace(cfg.Indexer.ScriptAddress)) {
			picked = &m
			break
		}
	}
	if picked == nil {
		picked = &matches[0]
	}
	if strings.TrimSpace(picked.DatumHash) == "" {
		return database.Client{}, errors.New("selected UTXO has no datum hash")
	}

	// Fetch datum (client information) using datum hash
	datumHex, err := k.Datum(ctx, parseHex(picked.DatumHash))
	if err != nil {
		return database.Client{}, fmt.Errorf("kupo datum: %w", err)
	}
	datumBytes, err := hex.DecodeString(parseHex(datumHex))
	if err != nil {
		return database.Client{}, fmt.Errorf("datum hex decode: %w", err)
	}

	// Decode CBOR into any first
	var v any
	if _, err := cbor.Decode(datumBytes, &v); err != nil {
		return database.Client{}, fmt.Errorf("datum cbor decode: %w", err)
	}

	// Remove CBOR wrappers (Tag/Constructors)
	fields, ok := toSlice(unwrapAll(v))
	if !ok || len(fields) != 3 {
		return database.Client{}, errors.New("unexpected client datum shape")
	}
	credentialRaw := unwrapAll(fields[0])
	credential, ok := credentialRaw.([]byte)
	if !ok || len(credential) == 0 {
		return database.Client{}, errors.New("invalid credential in client datum")
	}
	region, ok := toString(unwrapAll(fields[1]))
	if !ok || region == "" {
		return database.Client{}, errors.New("invalid region in client datum")
	}
	expirationMs, ok := toInt(unwrapAll(fields[2]))
	if !ok {
		return database.Client{}, errors.New("invalid expiration in client datum")
	}
	if credential == nil || region == "" {
		return database.Client{}, errors.New("invalid fields in client datum")
	}

	// Extract transaction hash
	txid, err := hex.DecodeString(parseHex(picked.TransactionID))
	if err != nil {
		return database.Client{}, fmt.Errorf("txid decode failure: %w", err)
	}
	assetNameBytes, _ := hex.DecodeString(targetAsset)

	return database.Client{
		AssetName:     assetNameBytes,
		Expiration:    time.UnixMilli(int64(expirationMs)),
		Credential:    credential,
		Region:        region,
		TxHash:        txid,
		TxOutputIndex: uint(picked.OutputIndex),
	}, nil
}
