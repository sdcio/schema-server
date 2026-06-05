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

package schema

import (
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/schema-server/pkg/config"
)

func dummySchema(t *testing.T) *Schema {
	t.Helper()
	sc, err := NewSchema(&config.SchemaConfig{
		Name:        "dummy",
		Vendor:      "dummy_vendor",
		Version:     "0.0.0",
		Files:       []string{"testdata/dummy"},
		Directories: []string{},
		Excludes:    []string{},
	})
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return sc
}

func containerSchemaForPath(t *testing.T, sc *Schema, path []string) *sdcpb.ContainerSchema {
	t.Helper()
	entry, err := sc.GetEntry(path)
	if err != nil {
		t.Fatalf("GetEntry(%v): %v", path, err)
	}
	elem, err := sc.SchemaElemFromYEntry(entry, false)
	if err != nil {
		t.Fatalf("SchemaElemFromYEntry(%v): %v", path, err)
	}
	c, ok := elem.Schema.(*sdcpb.SchemaElem_Container)
	if !ok {
		t.Fatalf("expected ContainerSchema for %v, got %T", path, elem.Schema)
	}
	return c.Container
}

// Test_uniqueConstraints_single verifies that a list with unique "ip port"
// compiles to UniqueConstraints: [{Elements: ["ip","port"]}].
func Test_uniqueConstraints_single(t *testing.T) {
	sc := dummySchema(t)
	c := containerSchemaForPath(t, sc, []string{"foo", "unique-one"})

	got := c.GetUniqueConstraints()
	if len(got) != 1 {
		t.Fatalf("expected 1 UniqueConstraint, got %d", len(got))
	}
	want := []string{"ip", "port"}
	if len(got[0].GetElements()) != len(want) {
		t.Fatalf("elements: got %v, want %v", got[0].GetElements(), want)
	}
	for i, e := range want {
		if got[0].GetElements()[i] != e {
			t.Errorf("element[%d]: got %q, want %q", i, got[0].GetElements()[i], e)
		}
	}
}

// Test_uniqueConstraints_none verifies that a list without unique compiles to
// an empty (nil/zero-length) UniqueConstraints slice.
func Test_uniqueConstraints_none(t *testing.T) {
	sc := dummySchema(t)
	c := containerSchemaForPath(t, sc, []string{"foo", "no-unique"})

	if len(c.GetUniqueConstraints()) != 0 {
		t.Errorf("expected empty UniqueConstraints, got %v", c.GetUniqueConstraints())
	}
}

// Test_uniqueConstraints_two verifies that two unique statements on the same
// list produce exactly two UniqueConstraint entries.
func Test_uniqueConstraints_two(t *testing.T) {
	sc := dummySchema(t)
	c := containerSchemaForPath(t, sc, []string{"foo", "unique-two"})

	got := c.GetUniqueConstraints()
	if len(got) != 2 {
		t.Fatalf("expected 2 UniqueConstraints, got %d: %v", len(got), got)
	}
	// first unique "ip port"
	want0 := []string{"ip", "port"}
	if len(got[0].GetElements()) != len(want0) {
		t.Fatalf("constraint[0] elements: got %v, want %v", got[0].GetElements(), want0)
	}
	for i, e := range want0 {
		if got[0].GetElements()[i] != e {
			t.Errorf("constraint[0].element[%d]: got %q, want %q", i, got[0].GetElements()[i], e)
		}
	}
	// second unique "name"
	want1 := []string{"name"}
	if len(got[1].GetElements()) != len(want1) {
		t.Fatalf("constraint[1] elements: got %v, want %v", got[1].GetElements(), want1)
	}
	if got[1].GetElements()[0] != "name" {
		t.Errorf("constraint[1].element[0]: got %q, want %q", got[1].GetElements()[0], "name")
	}
}

// Test_uniqueConstraints_container_node verifies that a plain container
// (non-list) node is not affected (no UniqueConstraints).
func Test_uniqueConstraints_container_node(t *testing.T) {
	sc := dummySchema(t)
	c := containerSchemaForPath(t, sc, []string{"foo"})

	if len(c.GetUniqueConstraints()) != 0 {
		t.Errorf("container node should have no UniqueConstraints, got %v", c.GetUniqueConstraints())
	}
}
