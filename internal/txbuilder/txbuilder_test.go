// Copyright 2026 Blink Labs Software
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

package txbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type fakeGenesisConfigClient struct {
	response []byte
	err      error
}

func (c fakeGenesisConfigClient) GenesisConfig(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(c.response), c.err
}

func TestOgmiosSystemStartReturnsZeroTimeOnGenesisError(t *testing.T) {
	ResetCachedSystemStart()
	t.Cleanup(ResetCachedSystemStart)
	wantErr := errors.New("genesis unavailable")

	got, err := ogmiosSystemStart(fakeGenesisConfigClient{err: wantErr})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ogmiosSystemStart() error = %v, want %v", err, wantErr)
	}
	if !got.IsZero() {
		t.Fatalf("ogmiosSystemStart() time = %v, want zero time", got)
	}
}

func TestOgmiosSystemStartReturnsZeroTimeOnInvalidGenesis(t *testing.T) {
	ResetCachedSystemStart()
	t.Cleanup(ResetCachedSystemStart)

	got, err := ogmiosSystemStart(fakeGenesisConfigClient{response: []byte("{")})
	if err == nil {
		t.Fatal("ogmiosSystemStart() error = nil, want JSON error")
	}
	if !got.Equal(time.Time{}) {
		t.Fatalf("ogmiosSystemStart() time = %v, want zero time", got)
	}
}
