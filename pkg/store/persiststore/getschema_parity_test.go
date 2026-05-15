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

package persiststore

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store"
	"github.com/sdcio/schema-server/pkg/store/memstore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

// assertGetSchemaParity checks that memstore and persiststore return the same outcome
// for the same GetSchemaRequest (memstore is the reference implementation).
func assertGetSchemaParity(t *testing.T, ms, ps store.Store, sk *sdcpb.Schema, req *sdcpb.GetSchemaRequest) {
	t.Helper()
	req = proto.Clone(req).(*sdcpb.GetSchemaRequest)
	if req.Schema == nil {
		req.Schema = sk
	}
	req.WithDescription = false

	ctx := context.Background()
	mRsp, mErr := ms.GetSchema(ctx, proto.Clone(req).(*sdcpb.GetSchemaRequest))
	pRsp, pErr := ps.GetSchema(ctx, proto.Clone(req).(*sdcpb.GetSchemaRequest))

	switch {
	case mErr != nil && pErr != nil:
		return
	case mErr != nil:
		t.Fatalf("memstore error %v, persiststore ok: %v", mErr, pRsp)
	case pErr != nil:
		t.Fatalf("persiststore error %v, memstore ok: %v", pErr, mRsp)
	}

	if !proto.Equal(mRsp.GetSchema(), pRsp.GetSchema()) {
		t.Fatalf("SchemaElem mismatch.\nmemstore: %s\npersist: %s",
			mRsp.GetSchema().String(), pRsp.GetSchema().String())
	}
}

func newParityStores(t *testing.T, cfg *config.SchemaConfig) (ms store.Store, ps *persistStore, sk *sdcpb.Schema) {
	t.Helper()
	scMem, err := schema.NewSchema(cfg)
	if err != nil {
		t.Fatalf("NewSchema (memstore): %v", err)
	}
	scPersist, err := schema.NewSchema(cfg)
	if err != nil {
		t.Fatalf("NewSchema (persiststore): %v", err)
	}
	ms = memstore.New()
	if err := ms.AddSchema(scMem); err != nil {
		t.Fatalf("memstore AddSchema: %v", err)
	}
	ps = newTestStore(t)
	if err := ps.AddSchema(scPersist); err != nil {
		t.Fatalf("persiststore AddSchema: %v", err)
	}
	sk = &sdcpb.Schema{Name: scMem.Name(), Vendor: scMem.Vendor(), Version: scMem.Version()}
	return ms, ps, sk
}

func TestGetSchema_MemstoreParity_AugmentFixture(t *testing.T) {
	td := filepath.Join("testdata", "augment")
	cfg := &config.SchemaConfig{
		Name:    "parity-augment",
		Vendor:  "test",
		Version: "0",
		Files:   []string{filepath.Join(td, "aug-base.yang"), filepath.Join(td, "aug-extra.yang")},
	}
	ms, ps, sk := newParityStores(t, cfg)

	cases := []struct {
		name string
		path *sdcpb.Path
	}{
		{
			name: "module_less_target_native_leaf",
			path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "target"}, {Name: "native-leaf"},
			}},
		},
		{
			name: "first_elem_module_prefixed",
			path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "aug-base:target"}, {Name: "native-leaf"},
			}},
		},
		{
			name: "augment_only_leaf_module_less",
			path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "target"}, {Name: "augment-only-leaf"},
			}},
		},
		{
			name: "first_prefixed_inner_module_less",
			path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "aug-base:target"}, {Name: "augment-only-leaf"},
			}},
		},
		{
			name: "inner_elem_has_module_prefix",
			path: &sdcpb.Path{Elem: []*sdcpb.PathElem{
				{Name: "aug-base:target"}, {Name: "aug-extra:augment-only-leaf"},
			}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertGetSchemaParity(t, ms, ps, sk, &sdcpb.GetSchemaRequest{Schema: sk, Path: tc.path})
		})
	}
}

func TestGetSchema_MemstoreParity_OriginDisambiguatesDupTopLevel(t *testing.T) {
	td := filepath.Join("testdata", "origin-dup")
	cfg := &config.SchemaConfig{
		Name:    "parity-origin-dup",
		Vendor:  "test",
		Version: "0",
		Files:   []string{filepath.Join(td, "moda.yang"), filepath.Join(td, "modb.yang")},
	}
	ms, ps, sk := newParityStores(t, cfg)

	basePath := &sdcpb.Path{Elem: []*sdcpb.PathElem{
		{Name: "dup"}, {Name: "marker"},
	}}

	t.Run("origin_moda", func(t *testing.T) {
		assertGetSchemaParity(t, ms, ps, sk, &sdcpb.GetSchemaRequest{
			Schema: sk,
			Path:   &sdcpb.Path{Origin: "moda", Elem: basePath.Elem},
		})
	})
	t.Run("origin_modb", func(t *testing.T) {
		assertGetSchemaParity(t, ms, ps, sk, &sdcpb.GetSchemaRequest{
			Schema: sk,
			Path:   &sdcpb.Path{Origin: "modb", Elem: basePath.Elem},
		})
	})
}
