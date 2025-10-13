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

import "sync"

var ep struct {
	mutex     sync.RWMutex
	ogmiosURL string
	kupoURL   string
}

// vpn cli override Ogmios/Kupo endpoints at runtime.
func SetChainEndpoints(ogmiosURL, kupoURL string) {
	ep.mutex.Lock()
	defer ep.mutex.Unlock()
	if ogmiosURL != "" {
		ep.ogmiosURL = ogmiosURL
		ResetCachedSystemStart()
	}
	if kupoURL != "" {
		ep.kupoURL = kupoURL
	}
}

func getChainEndpoints() (ogmiosURL, kupoURL string) {
	ep.mutex.RLock()
	defer ep.mutex.RUnlock()
	return ep.ogmiosURL, ep.kupoURL
}
