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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func newPersistTestStore(t *testing.T) *persistStore {
	t.Helper()
	dir := t.TempDir()
	db, err := badger.Open(badger.DefaultOptions(dir).WithLogger(nil))
	if err != nil {
		t.Fatalf("badger open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &persistStore{db: db}
}

func TestGetSchema_ShallowRouterAmbiguous(t *testing.T) {
	ps := newPersistTestStore(t)
	sk := store.SchemaKey{Name: "M", Vendor: "V", Version: "1"}
	insertSchemaMeta(t, ps, sk)
	insertRootEntry(t, ps, sk, []string{"cisco-router-isis", "cisco-router-ospf", "cisco-router-static"})

	router := &sdcpb.SchemaElem{Schema: &sdcpb.SchemaElem_Container{Container: &sdcpb.ContainerSchema{Name: "router"}}}
	for _, mod := range []string{"cisco-router-isis", "cisco-router-ospf", "cisco-router-static"} {
		insertEntry(t, ps, sk, []string{mod, "router"}, proto.Clone(router).(*sdcpb.SchemaElem))
	}

	_, err := ps.GetSchema(context.Background(), &sdcpb.GetSchemaRequest{
		Schema: &sdcpb.Schema{Name: sk.Name, Vendor: sk.Vendor, Version: sk.Version},
		Path:   &sdcpb.Path{Elem: []*sdcpb.PathElem{{Name: "router"}}},
	})
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
}

func insertSchemaMeta(t *testing.T, ps *persistStore, sk store.SchemaKey) {
	t.Helper()
	key := buildSchemaKey(sk)
	if err := ps.db.Update(func(txn *badger.Txn) error { return txn.Set(key, []byte(`{}`)) }); err != nil {
		t.Fatalf("schema meta: %v", err)
	}
}

func insertRootEntry(t *testing.T, ps *persistStore, sk store.SchemaKey, modules []string) {
	t.Helper()
	root := &sdcpb.SchemaElem{
		Schema: &sdcpb.SchemaElem_Container{Container: &sdcpb.ContainerSchema{
			Name:     schema.RootName,
			Children: modules,
		}},
	}
	b, err := proto.Marshal(root)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	key := buildEntryKey(sk, []string{schema.RootName})
	if err := ps.db.Update(func(txn *badger.Txn) error { return txn.Set(key, b) }); err != nil {
		t.Fatalf("root: %v", err)
	}
}

func insertEntry(t *testing.T, ps *persistStore, sk store.SchemaKey, keyPath []string, se *sdcpb.SchemaElem) {
	t.Helper()
	b, err := proto.Marshal(se)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	key := buildEntryKey(sk, keyPath)
	if err := ps.db.Update(func(txn *badger.Txn) error { return txn.Set(key, b) }); err != nil {
		t.Fatalf("entry: %v", err)
	}
}
