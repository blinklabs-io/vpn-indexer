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

package client

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/blinklabs-io/vpn-indexer/internal/ca"
	"github.com/blinklabs-io/vpn-indexer/internal/config"
	"golang.org/x/crypto/blake2b"
)

const profileTemplate = `
client
dev tun
proto tcp
remote %s %d
nobind
persist-tun

<cert>
%s
</cert>

<key>
%s
</key>

<ca>
%s
</ca>
`

type Client struct {
	config    *config.Config
	ca        *ca.Ca
	assetName []byte
	id        string
}

func New(cfg *config.Config, caObj *ca.Ca, assetName []byte) *Client {
	return &Client{
		config:    cfg,
		ca:        caObj,
		assetName: assetName,
	}
}

func (c *Client) Generate(host string, port int) (string, error) {
	if ok, err := c.ProfileExists(); err != nil {
		return "", err
	} else if ok {
		return c.identifier(), nil
	}
	// Generate certs for client
	certs, err := c.ca.GenerateClientCert(c.identifier())
	if err != nil {
		return "", err
	}
	// Generate profile from template
	profile := fmt.Sprintf(
		profileTemplate,
		host,
		port,
		certs.Cert,
		certs.Key,
		certs.CaCert,
	)
	// Upload profile to S3
	svc, err := c.createS3Client()
	if err != nil {
		return "", err
	}
	_, err = svc.PutObject(
		context.TODO(),
		&s3.PutObjectInput{
			Bucket: aws.String(c.config.S3.ClientBucket),
			Key:    aws.String(c.profileKey()),
			Body:   strings.NewReader(profile),
		},
	)
	if err != nil {
		return "", err
	}
	return c.identifier(), nil
}

func (c *Client) ProfileExists() (bool, error) {
	svc, err := c.createS3Client()
	if err != nil {
		return false, err
	}
	_, err = svc.HeadObject(
		context.TODO(),
		&s3.HeadObjectInput{
			Bucket: aws.String(c.config.S3.ClientBucket),
			Key:    aws.String(c.profileKey()),
		},
	)
	if err != nil {
		// Check for explicit "No such key" error
		var nfErr *s3types.NotFound
		if errors.As(err, &nfErr) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Client) PresignedUrl() (string, error) {
	svc, err := c.createS3Client()
	if err != nil {
		return "", err
	}
	presignClient := s3.NewPresignClient(svc)
	request, err := presignClient.PresignGetObject(
		context.Background(),
		&s3.GetObjectInput{
			Bucket: aws.String(c.config.S3.ClientBucket),
			Key:    aws.String(c.profileKey()),
		}, func(opts *s3.PresignOptions) {
			opts.Expires = time.Duration(5 * int64(time.Minute))
		},
	)
	if err != nil {
		return "", err
	}
	return request.URL, nil
}

func (c *Client) profileKey() string {
	return fmt.Sprintf(
		"%s%s.ovpn",
		c.config.S3.ClientKeyPrefix,
		c.identifier(),
	)
}

func (c *Client) createS3Client() (*s3.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}
	var clientOpts []func(o *s3.Options)
	if c.config.S3.Endpoint != "" {
		clientOpts = append(
			clientOpts,
			func(o *s3.Options) {
				o.BaseEndpoint = aws.String(c.config.S3.Endpoint)
				// This is needed for local minio
				o.UsePathStyle = true
			},
		)
	}
	client := s3.NewFromConfig(cfg, clientOpts...)
	return client, nil
}

func (c *Client) identifier() string {
	// Returned cached response
	if c.id != "" {
		return c.id
	}
	// Create blake2b-256 hash from client name and encode as hex
	hasher, _ := blake2b.New(32, nil)
	hasher.Write(c.assetName)
	hash := hasher.Sum(nil)
	c.id = hex.EncodeToString(hash)
	return c.id
}
