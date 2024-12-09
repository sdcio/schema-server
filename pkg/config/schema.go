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
	"errors"
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

const (
	StoreTypePersistent = "persistent"
	StoreTypeMemory     = "memory"
)

var (
	ImportedMods = []string{"ietf", "iana"}
)

type SchemaStoreConfig struct {
	Type    string                         `yaml:"type,omitempty" json:"type,omitempty"`
	Path    string                         `yaml:"path,omitempty" json:"path,omitempty"`
	Cache   *SchemaPersistStoreCacheConfig `json:"cache,omitempty"`
	Schemas []*SchemaConfig                `yaml:"schemas,omitempty" json:"schemas,omitempty"`
}

type SchemaPersistStoreCacheConfig struct {
	TTL             time.Duration `yaml:"ttl,omitempty" json:"ttl,omitempty"`
	Capacity        uint64        `yaml:"capacity,omitempty" json:"capacity,omitempty"`
	WithDescription bool          `yaml:"with-description,omitempty" json:"with-description,omitempty"`
}

type SchemaConfig struct {
	Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
	Vendor      string   `yaml:"vendor,omitempty" json:"vendor,omitempty"`
	Version     string   `yaml:"version,omitempty" json:"version,omitempty"`
	Files       []string `yaml:"files,omitempty" json:"files,omitempty"`
	Directories []string `yaml:"directories,omitempty" json:"directories,omitempty"`
	Excludes    []string `yaml:"excludes,omitempty" json:"excludes,omitempty"`
}

func (sc *SchemaConfig) validateSetDefaults() error {
	if sc.Vendor == "" || sc.Version == "" {
		return errors.New("schema name, vendor and version should be set")
	}
	return nil
}

func (sc *SchemaConfig) GetSchema() *sdcpb.Schema {
	return &sdcpb.Schema{
		Name:    sc.Name,
		Vendor:  sc.Vendor,
		Version: sc.Version,
	}
}
