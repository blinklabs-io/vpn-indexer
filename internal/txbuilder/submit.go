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

package txbuilder

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/blinklabs-io/vpn-indexer/internal/config"
)

func SubmitTx(txRawBytes []byte) (string, error) {
	cfg := config.GetConfig()
	client := createHTTPClient()
	body := bytes.NewBuffer(txRawBytes)
	req, err := http.NewRequest(
		http.MethodPost,
		cfg.TxBuilder.SubmitUrl,
		body,
	)
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/cbor")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	if resp.StatusCode == http.StatusAccepted {
		return string(respBody), nil
	}
	return "", errors.New("empty body returned")
}

// createHTTPClient with custom timeout
func createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			DisableCompression:    false,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
