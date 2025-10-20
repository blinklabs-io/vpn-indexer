package main

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type plan struct {
	Duration int
	Price    int
}

func decodeRefDatumFlexible(datumCBOR []byte) ([]plan, []string, error) {
	// Decode CBOR into any first
	var v any
	_, err := cbor.Decode(datumCBOR, &v)
	if err != nil {
		return nil, nil, fmt.Errorf("cbor decode: %w", err)
	}

	// Remove CBOR wrappers (Tag/Constructors)
	v = unwrapAll(v)

	// We expect either:
	//   [ ....,...]   OR   Constructor( [ plans, regions, ... ] )
	seq, ok := toSlice(v)
	if !ok {
		return nil, nil, errors.New("refdatum: top-level is not a list/constructor")
	}

	// conisdered first list of (int,int) as plans and the first "list of string" as regions.
	var rawPlans any
	var rawRegions any
	for _, e := range seq {
		eu := unwrapAll(e)
		if rawPlans == nil {
			if ps, ok := isListOfIntPairs(eu); ok {
				rawPlans = ps
				continue
			}
		}
		if rawRegions == nil {
			if rs, ok := isListOfStrings(eu); ok {
				rawRegions = rs
				continue
			}
		}
	}
	if rawPlans == nil {
		return nil, nil, fmt.Errorf("refdatum: plans not found")
	}
	pairs := rawPlans.([][]int)
	outPlans := make([]plan, 0, len(pairs))
	for _, p := range pairs {
		outPlans = append(outPlans, plan{Duration: p[0], Price: p[1]})
	}

	if rawRegions == nil {
		return nil, nil, fmt.Errorf("refdatum: regions not found")
	}
	outRegions := rawRegions.([]string)

	return outPlans, outRegions, nil
}

// Removes cbor.Tag and cbor.Constructor.
func unwrapAll(x any) any {
	for {
		switch t := x.(type) {
		case cbor.Tag:
			x = t.Content
			continue
		case *cbor.Constructor:
			fields := t.Fields()
			if len(fields) == 1 {
				x = fields[0]
				continue
			}
			x = fields
			continue
		case cbor.Constructor:
			if len(t.Fields()) == 1 {
				x = t.Fields()[0]
				continue
			}
			x = t.Fields()
			continue
		default:
			return x
		}
	}
}

// toSlice returns v as []any
func toSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

// Read a list as [[int,int], ...] even if each pair is []any.
func isListOfIntPairs(v any) ([][]int, bool) {
	items, ok := toSlice(v)
	if !ok || len(items) == 0 {
		return nil, false
	}
	out := make([][]int, 0, len(items))
	for _, it := range items {
		// unwrap constructor to CBOR fields
		it = unwrapAll(it)
		pair, ok := toSlice(it)
		if !ok || len(pair) != 2 {
			return nil, false
		}
		a, okA := toInt(unwrapAll(pair[0]))
		b, okB := toInt(unwrapAll(pair[1]))
		if !okA || !okB {
			return nil, false
		}
		out = append(out, []int{a, b})
	}
	return out, true
}

// Can be either strings or []byte (UTF-8) per element.
func isListOfStrings(v any) ([]string, bool) {
	items, ok := toSlice(v)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		it = unwrapAll(it)
		switch s := it.(type) {
		case string:
			out = append(out, s)
		case []byte:
			out = append(out, string(s))
		default:
			return nil, false
		}
	}
	return out, true
}

// convert any CBOR number into an int.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case uint64:
		return int(n), true
	case uint:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
		return 0, false
	default:
		return 0, false
	}
}
