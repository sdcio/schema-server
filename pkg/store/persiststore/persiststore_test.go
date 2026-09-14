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
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/proto"
)

//
// ---------- helpers ----------
//

func newTestStore(t *testing.T) *persistStore {
	t.Helper()

	dir := t.TempDir()
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nil))
	if err != nil {
		t.Fatalf("failed to open badger: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })

	return &persistStore{db: db}
}

func testSchemaKey() store.SchemaKey {
	return store.SchemaKey{
		Name:    "M",
		Vendor:  "V",
		Version: "1",
	}
}

func insertSchemaMeta(t *testing.T, ps *persistStore, sk store.SchemaKey) {
	t.Helper()

	key := buildSchemaKey(sk)
	err := ps.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, []byte(`{}`))
	})
	if err != nil {
		t.Fatalf("failed inserting schema meta: %v", err)
	}
}

func insertRootEntry(t *testing.T, ps *persistStore, sk store.SchemaKey, modules []string) {
	t.Helper()

	// Create a root container listing available modules as children
	root := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{Container: &sdcpb.ContainerSchema{
			Name:     schema.RootName,
			Children: modules,
		}},
	}
	b, err := proto.Marshal(root)
	if err != nil {
		t.Fatalf("marshal root: %v", err)
	}
	key := buildEntryKey(sk, []string{schema.RootName})
	if err := ps.db.Update(func(txn *badger.Txn) error { return txn.Set(key, b) }); err != nil {
		t.Fatalf("insert root failed: %v", err)
	}
}

func insertEntry(t *testing.T, ps *persistStore, sk store.SchemaKey, keyPath []string, se *sdcpb.SchemaElem) {
	t.Helper()
	b, err := proto.Marshal(se)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	key := buildEntryKey(sk, keyPath)
	if err := ps.db.Update(func(txn *badger.Txn) error { return txn.Set(key, b) }); err != nil {
		t.Fatalf("insert entry failed: %v", err)
	}
}

//
// ---------- helper function tests ----------
//

func TestSchemaKeyString(t *testing.T) {
	sk := store.SchemaKey{Name: "n", Vendor: "v", Version: "1"}
	if got := schemaKeyString(sk); got != "n@v@1" {
		t.Fatalf("unexpected schemaKeyString: %q", got)
	}
}

func TestStripPrefix(t *testing.T) {
	cases := map[string]string{
		"a":       "a",
		"m:a":     "a",
		"foo:bar": "bar",
	}

	for in, exp := range cases {
		if got := stripPrefix(in); got != exp {
			t.Fatalf("stripPrefix(%q)=%q, want %q", in, got, exp)
		}
	}
}

func TestHasPrefix(t *testing.T) {
	if !hasPrefix("m:a") {
		t.Fatalf("expected prefix")
	}
	if hasPrefix("a") {
		t.Fatalf("unexpected prefix")
	}
}

//
// ---------- HasSchema tests ----------
//

func TestHasSchema(t *testing.T) {
	ps := newTestStore(t)
	sk := testSchemaKey()

	if ps.HasSchema(sk) {
		t.Fatalf("schema should not exist")
	}

	insertSchemaMeta(t, ps, sk)

	if !ps.HasSchema(sk) {
		t.Fatalf("schema should exist")
	}
}

//
// ---------- GetSchema tests (negative + strict) ----------
//

func TestGetSchema_UnknownSchema(t *testing.T) {
	ps := newTestStore(t)

	_, err := ps.GetSchema(context.Background(), &sdcpb.GetSchemaRequest{
		Schema: &sdcpb.Schema{
			Name:    "M",
			Vendor:  "V",
			Version: "1",
		},
	})
	if err == nil {
		t.Fatalf("expected error for unknown schema")
	}
}

func TestGetSchema_StrictPrefixRejected(t *testing.T) {
	// Replace strict-prefix behavior with validation of unknown module hints
	ps := newTestStore(t)
	sk := testSchemaKey()
	insertSchemaMeta(t, ps, sk)
	// Insert root with one known module
	insertRootEntry(t, ps, sk, []string{"known"})

	_, err := ps.GetSchema(context.Background(), &sdcpb.GetSchemaRequest{
		Schema: &sdcpb.Schema{Name: sk.Name, Vendor: sk.Vendor, Version: sk.Version},
		Path:   &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "unknown:foo"}}},
	})
	if err == nil {
		t.Fatalf("expected error for unknown module prefix")
	}
}

func TestGetSchema_RootSchemaMissingEntry(t *testing.T) {
	ps := newTestStore(t)
	sk := testSchemaKey()
	insertSchemaMeta(t, ps, sk)

	_, err := ps.GetSchema(context.Background(), &sdcpb.GetSchemaRequest{
		Schema: &sdcpb.Schema{
			Name:    sk.Name,
			Vendor:  sk.Vendor,
			Version: sk.Version,
		},
	})
	if err == nil {
		t.Fatalf("expected error due to missing root entry")
	}
}

func TestGetSchema_ModuleLessPathResolves(t *testing.T) {
	ps := newTestStore(t)
	sk := testSchemaKey()
	insertSchemaMeta(t, ps, sk)
	// Set up root with module list and a concrete entry under that module
	insertRootEntry(t, ps, sk, []string{"ietf-nss"})
	// Create container entry for ietf-nss:network-instances
	se := &sdcpb.SchemaElem{Schema: &sdcpb.SchemaElem_Container{Container: &sdcpb.ContainerSchema{Name: "network-instances"}}}
	insertEntry(t, ps, sk, []string{"ietf-nss", "network-instances"}, se)

	rsp, err := ps.GetSchema(context.Background(), &sdcpb.GetSchemaRequest{
		Schema: &sdcpb.Schema{Name: sk.Name, Vendor: sk.Vendor, Version: sk.Version},
		Path:   &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "network-instances"}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := rsp.GetSchema().GetContainer().GetName()
	if got != "network-instances" {
		t.Fatalf("unexpected schema name: %q", got)
	}
}

func TestGetSchema_ModulePrefixedPathResolves(t *testing.T) {
	ps := newTestStore(t)
	sk := testSchemaKey()
	insertSchemaMeta(t, ps, sk)
	insertRootEntry(t, ps, sk, []string{"ietf-nss"})
	se := &sdcpb.SchemaElem{Schema: &sdcpb.SchemaElem_Container{Container: &sdcpb.ContainerSchema{Name: "network-instances"}}}
	insertEntry(t, ps, sk, []string{"ietf-nss", "network-instances"}, se)

	rsp, err := ps.GetSchema(context.Background(), &sdcpb.GetSchemaRequest{
		Schema: &sdcpb.Schema{Name: sk.Name, Vendor: sk.Vendor, Version: sk.Version},
		Path:   &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "ietf-nss:network-instances"}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := rsp.GetSchema().GetContainer().GetName()
	if got != "network-instances" {
		t.Fatalf("unexpected schema name: %q", got)
	}
}
