// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
)

const (
	defaultServerAddress = ":55000"
	defaultMessageSize   = 4 * 1024 * 1024
	defaultRPCTimeout    = time.Minute
)

type Config struct {
	GRPCServer  *GRPCServer        `yaml:"grpc-server,omitempty" json:"grpc-server,omitempty"`
	SchemaStore *SchemaStoreConfig `yaml:"schema-store,omitempty" json:"schema-store,omitempty"`
	// Schemas     []*SchemaConfig    `yaml:"schemas,omitempty" json:"schemas,omitempty"`
	Prometheus *PromConfig `yaml:"prometheus,omitempty" json:"prometheus,omitempty"`
}

type TLS struct {
	CA         string `yaml:"ca,omitempty" json:"ca,omitempty"`
	Cert       string `yaml:"cert,omitempty" json:"cert,omitempty"`
	Key        string `yaml:"key,omitempty" json:"key,omitempty"`
	SkipVerify bool   `yaml:"skip-verify,omitempty" json:"skip-verify,omitempty"`
}

func New(file string) (*Config, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	c := new(Config)
	err = yaml.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}
	err = c.validateSetDefaults()
	return c, err
}

func (c *Config) validateSetDefaults() error {
	if c.GRPCServer == nil {
		c.GRPCServer = &GRPCServer{}
	}
	if c.GRPCServer.Address == "" {
		c.GRPCServer.Address = defaultServerAddress
	}
	if c.GRPCServer.MaxRecvMsgSize <= 0 {
		c.GRPCServer.MaxRecvMsgSize = defaultMessageSize
	}
	if c.GRPCServer.RPCTimeout <= 0 {
		c.GRPCServer.RPCTimeout = defaultRPCTimeout
	}
	if c.SchemaStore == nil {
		c.SchemaStore = &SchemaStoreConfig{}
		return nil
	}
	var err error
	for _, sc := range c.SchemaStore.Schemas {
		if err = sc.validateSetDefaults(); err != nil {
			return err
		}
	}
	return nil
}

type RemoteSchemaServer struct {
	Address string `yaml:"address,omitempty" json:"address,omitempty"`
	TLS     *TLS   `yaml:"tls,omitempty" json:"tls,omitempty"`
}

type GRPCServer struct {
	Address        string        `yaml:"address,omitempty" json:"address,omitempty"`
	TLS            *TLS          `yaml:"tls,omitempty" json:"tls,omitempty"`
	SchemaServer   *SchemaServer `yaml:"schema-server,omitempty" json:"schema-server,omitempty"`
	MaxRecvMsgSize int           `yaml:"max-recv-msg-size,omitempty" json:"max-recv-msg-size,omitempty"`
	RPCTimeout     time.Duration `yaml:"rpc-timeout,omitempty" json:"rpc-timeout,omitempty"`
}

func (t *TLS) NewConfig(ctx context.Context) (*tls.Config, error) {
	tlsCfg := &tls.Config{InsecureSkipVerify: t.SkipVerify}
	if t.CA != "" {
		ca, err := os.ReadFile(t.CA)
		if err != nil {
			return nil, fmt.Errorf("failed to read client CA cert: %w", err)
		}
		if len(ca) != 0 {
			caCertPool := x509.NewCertPool()
			caCertPool.AppendCertsFromPEM(ca)
			tlsCfg.RootCAs = caCertPool
		}
	}

	if t.Cert != "" && t.Key != "" {
		certWatcher, err := certwatcher.New(t.Cert, t.Key)
		if err != nil {
			return nil, err
		}

		go func() {
			if err := certWatcher.Start(ctx); err != nil {
				log.Errorf("certificate watcher error: %v", err)
			}
		}()
		tlsCfg.GetCertificate = certWatcher.GetCertificate
	}
	return tlsCfg, nil
}

type SchemaServer struct {
	// Enabled          bool   `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	SchemasDirectory string `yaml:"schemas-directory,omitempty" json:"schemas-directory,omitempty"`
}
