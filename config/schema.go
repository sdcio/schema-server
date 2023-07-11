package config

import (
	"errors"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
)

type SchemaConfig struct {
	Name        string   `json:"name,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Version     string   `json:"version,omitempty"`
	Files       []string `json:"files,omitempty"`
	Directories []string `json:"directories,omitempty"`
	Excludes    []string `json:"excludes,omitempty"`
}

func (sc *SchemaConfig) validateSetDefaults() error {
	if sc.Name == "" || sc.Vendor == "" || sc.Version == "" {
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
