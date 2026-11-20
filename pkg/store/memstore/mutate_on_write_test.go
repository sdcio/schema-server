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

package memstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sdcio/schema-server/pkg/store/memstore"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func TestMemStore_GetSchema_MutateOnWrite(t *testing.T) {
	ctx := context.Background()
	st := memstore.New()
	t.Cleanup(func() { _ = st.Close() })

	scRef := &sdcpb.Schema{
		Name:    "mow",
		Vendor:  "test",
		Version: "1.0.0",
	}

	_, err := st.CreateSchema(ctx, &sdcpb.CreateSchemaRequest{
		Schema: scRef,
		File:   []string{filepath.Join("..", "..", "schema", "testdata", "mutate-on-write")},
	})
	if err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	tests := []struct {
		name  string
		elems []string
		want  bool
	}{
		{name: "container", elems: []string{"system"}, want: false},
		{name: "custom prefix deviate add leaf", elems: []string{"system", "overlay-secret"}, want: true},
		{name: "custom prefix deviate add leaf-list", elems: []string{"system", "overlay-secret-list"}, want: true},
		{name: "openconfig hashed extension", elems: []string{"system", "oc-hashed-ext"}, want: true},
		{name: "srl inline SecretKey", elems: []string{"system", "srl-inline-secret-key"}, want: true},
		{name: "srl cleartext routing-password", elems: []string{"system", "srl-routing-pw"}, want: false},
		{name: "union with crypt member", elems: []string{"system", "union-with-crypt-typedef"}, want: true},
		{name: "2-hop leafref to crypt-hash", elems: []string{"system", "leafref-hop2"}, want: true},
		{name: "cyclic leafref", elems: []string{"system", "cycle-a"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toPathRsp, err := st.ToPath(ctx, &sdcpb.ToPathRequest{
				Schema:      scRef,
				PathElement: tt.elems,
			})
			if err != nil {
				t.Fatalf("ToPath(%v): %v", tt.elems, err)
			}
			rsp, err := st.GetSchema(ctx, &sdcpb.GetSchemaRequest{
				Schema: scRef,
				Path:   toPathRsp.GetPath(),
			})
			if err != nil {
				t.Fatalf("GetSchema(%v): %v", tt.elems, err)
			}
			if got := rsp.GetSchema().IsMutateOnWrite(); got != tt.want {
				t.Errorf("GetSchema(%v).IsMutateOnWrite() = %v, want %v", tt.elems, got, tt.want)
			}
		})
	}
}
