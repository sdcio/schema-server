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

package store

import (
	"context"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"

	"github.com/iptecharch/schema-server/pkg/schema"
)

type SchemaKey struct {
	Name, Vendor, Version string
}

func (sck SchemaKey) String() string {
	return sck.Name + "@" + sck.Vendor + "@" + sck.Version
}

func Key(s *schema.Schema) SchemaKey {
	return SchemaKey{
		Name:    s.Name(),
		Vendor:  s.Vendor(),
		Version: s.Version(),
	}
}

type Store interface {
	HasSchema(scKey SchemaKey) bool
	ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error)
	GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error)
	CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error)
	ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error)
	DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error)
	AddSchema(sc *schema.Schema) error

	GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error)
	GetSchemaElements(ctx context.Context, req *sdcpb.GetSchemaRequest) (chan *sdcpb.SchemaElem, error)
	ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error)
	ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error)
}
