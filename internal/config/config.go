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
	"time"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Logging   LoggingConfig   `yaml:"logging"`
	Metrics   MetricsConfig   `yaml:"metrics"`
	Debug     DebugConfig     `yaml:"debug"`
	Indexer   IndexerConfig   `yaml:"indexer"`
	Database  DatabaseConfig  `yaml:"database"`
	Ca        CaConfig        `yaml:"ca"`
	S3        S3Config        `yaml:"s3"`
	Vpn       VpnConfig       `yaml:"vpn"`
	Crl       CrlConfig       `yaml:"crl"`
	Api       ApiConfig       `yaml:"api"`
	TxBuilder TxBuilderConfig `yaml:"txBuilder"`
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
	ReferenceToken     string `yaml:"referenceToken" envconfig:"INDEXER_REFERENCE_TOKEN"`
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
	Endpoint        string `yaml:"endpoint" envconfig:"S3_ENDPOINT"`
}

type VpnConfig struct {
	Domain string `yaml:"domain"`
	Port   int    `yaml:"port"`
}

type CrlConfig struct {
	UpdateInterval     time.Duration `yaml:"updateInterval" envconfig:"CRL_UPDATE_INTERVAL"`
	RevokeSerials      []string      `yaml:"revokeSerials" envconfig:"CRL_REVOKE_SERIALS"`
	RevokeTime         time.Time     `yaml:"revokeTime" envconfig:"CRL_REVOKE_TIME"`
	ConfigMapNamespace string        `yaml:"configMapNamespace" envconfig:"CRL_CONFIGMAP_NAMESPACE"`
	ConfigMapName      string        `yaml:"configMapName" envconfig:"CRL_CONFIGMAP_NAME"`
	ConfigMapKey       string        `yaml:"configMapKey" envconfig:"CRL_CONFIGMAP_KEY"`
}

type ApiConfig struct {
	ListenAddress  string `yaml:"address" envconfig:"API_LISTEN_ADDRESS"`
	ListenPort     uint   `yaml:"port"    envconfig:"API_LISTEN_PORT"`
	LogHealthcheck bool   `yaml:"logHealthcheck" envconfig:"API_LOG_HEALTHCHECK"`
}

type TxBuilderConfig struct {
	KupoUrl         string `yaml:"kupoUrl" envconfig:"TXBUILDER_KUPO_URL"`
	OgmiosUrl       string `yaml:"ogmiosUrl" envconfig:"TXBUILDER_OGMIOS_URL"`
	ProviderAddress string `yaml:"providerAddress" envconfig:"TXBUILDER_PROVIDER_ADDRESS"`
	ScriptRefInput  string `yaml:"scriptRefInput" envconfig:"TXBUILDER_SCRIPT_REF_INPUT"`
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
		Network: "preprod",
		// NOTE: these values correspond to the block before the reference token and/or script used below appear on-chain
		IntersectSlot:  97_153_054,
		IntersectHash:  "833506cade5ebd129065f76bc13260147e185797e0f265658499b6d57fe5b584",
		ScriptAddress:  "addr_test1zrmx2wvrdqurqgp2wg4v8v9g27zugxezyt75as9zkycyda2jduk3c6ecrpkrk8qqlr4ep37cx03ytlcn70n93zyemj6s4mgt63",
		ReferenceToken: "85b95fee1b7cf6f2a7dca818e77b901ff3c1b6aae4473219cf56902b.61646d696e",
	},
	Database: DatabaseConfig{
		Directory: "./.vpn-indexer",
	},
	Vpn: VpnConfig{
		Domain: "test.domain",
		Port:   443,
	},
	Crl: CrlConfig{
		UpdateInterval: 60 * time.Minute,
		// The actual doesn't matter, but we want a consistent value for any custom revoked certs
		RevokeTime: time.Date(2025, 06, 11, 15, 45, 03, 0, time.UTC),
	},
	Api: ApiConfig{
		ListenPort:     8080,
		LogHealthcheck: true,
	},
	TxBuilder: TxBuilderConfig{
		// NOTE: this shares a stake key with the indexer script address
		ProviderAddress: "addr_test1qpjwevqy6mh5hsnudjgpgrtfjwwxdtl7d73e9u0kxg9453jjduk3c6ecrpkrk8qqlr4ep37cx03ytlcn70n93zyemj6sasxnj5",
		ScriptRefInput:  "3588c6f7d5fbd80b6bfc2ad2d6ad595a7fc750361909f89cf2e911519d1fbdea#0",
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
