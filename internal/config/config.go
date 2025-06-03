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

package config

import (
	"fmt"
	"os"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Logging  LoggingConfig  `yaml:"logging"`
	Metrics  MetricsConfig  `yaml:"metrics"`
	Debug    DebugConfig    `yaml:"debug"`
	Indexer  IndexerConfig  `yaml:"indexer"`
	Database DatabaseConfig `yaml:"database"`
	Ca       CaConfig       `yaml:"ca"`
	S3       S3Config       `yaml:"s3"`
}

type LoggingConfig struct {
	Debug bool `yaml:"debug"    envconfig:"LOGGING_DEBUG"`
}

type DebugConfig struct {
	ListenAddress string `yaml:"address" envconfig:"DEBUG_ADDRESS"`
	ListenPort    uint   `yaml:"port"    envconfig:"DEBUG_PORT"`
}

type MetricsConfig struct {
	ListenAddress string `yaml:"address" envconfig:"METRICS_ADDRESS"`
	ListenPort    uint   `yaml:"port"    envconfig:"METRICS_PORT"`
}

type IndexerConfig struct {
	Network            string `yaml:"network"       envconfig:"INDEXER_NETWORK"`
	NetworkMagic       uint32 `yaml:"networkMagic"  envconfig:"INDEXER_NETWORK_MAGIC"`
	Address            string `yaml:"address"       envconfig:"INDEXER_TCP_ADDRESS"`
	SocketPath         string `yaml:"socketPath"    envconfig:"INDEXER_SOCKET_PATH"`
	IntersectHash      string `yaml:"interceptHash" envconfig:"INDEXER_INTERSECT_HASH"`
	IntersectSlot      uint64 `yaml:"interceptSlot" envconfig:"INDEXER_INTERSECT_SLOT"`
	ScriptAddress      string `yaml:"scriptAddress" envconfig:"INDEXER_SCRIPT_ADDRESS"`
	DelayConfirmations uint   `yaml:"delayConfirmations" envconfig:"INDEXER_DELAY_CONFIRMATIONS"`
}

type DatabaseConfig struct {
	Directory string `yaml:"dir" envconfig:"DATABASE_DIR"`
}

type CaConfig struct {
	Cert           string `yaml:"cert" envconfig:"CA_CERT"`
	CertFile       string `yaml:"certFile" envconfig:"CA_CERT_FILE"`
	Key            string `yaml:"key" envconfig:"CA_KEY"`
	KeyFile        string `yaml:"keyFile" envconfig:"CA_KEY_FILE"`
	Passphrase     string `yaml:"passphrase" envconfig:"CA_PASSPHRASE"`
	PassphraseFile string `yaml:"passphraseFile" envconfig:"CA_PASSPHRASE_FILE"`
}

type S3Config struct {
	ClientBucket    string `yaml:"clientBucket" envconfig:"S3_CLIENT_BUCKET"`
	ClientKeyPrefix string `yaml:"clientKeyPrefix" envconfig:"S3_CLIENT_KEY_PREFIX"`
}

// Singleton config instance with default values
var globalConfig = &Config{
	Logging: LoggingConfig{
		Debug: false,
	},
	Debug: DebugConfig{
		ListenAddress: "localhost",
		ListenPort:    0,
	},
	Metrics: MetricsConfig{
		ListenAddress: "",
		ListenPort:    8081,
	},
	Indexer: IndexerConfig{
		Network: "mainnet",
		// NOTE: these values were the current tip at the time this code was written
		// The user should provide more appropriate values, especially on a network other than mainnet
		IntersectSlot: 156_204_633,
		IntersectHash: "7a5708b6a34c389991474273817847aadfc0097ca57cccffd1c2fb4c6c76bbec",
	},
	Database: DatabaseConfig{
		Directory: "./.vpn-indexer",
	},
}

func Load(configFile string) (*Config, error) {
	// Load config file as YAML if provided
	if configFile != "" {
		buf, err := os.ReadFile(configFile)
		if err != nil {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		err = yaml.Unmarshal(buf, globalConfig)
		if err != nil {
			return nil, fmt.Errorf("error parsing config file: %w", err)
		}
	}
	// Load config values from environment variables
	// We use "dummy" as the app name here to (mostly) prevent picking up env
	// vars that we hadn't explicitly specified in annotations above
	err := envconfig.Process("dummy", globalConfig)
	if err != nil {
		return nil, fmt.Errorf("error processing environment: %w", err)
	}
	return globalConfig, nil
}

// GetConfig returns the global config instance
func GetConfig() *Config {
	return globalConfig
}
