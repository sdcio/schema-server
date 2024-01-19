package config

import (
	"errors"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
)

type SchemaStoreConfig struct {
	Type    string          `yaml:"type,omitempty" json:"type,omitempty"`
	Path    string          `yaml:"path,omitempty" json:"path,omitempty"`
	Schemas []*SchemaConfig `yaml:"schemas,omitempty" json:"schemas,omitempty"`
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
