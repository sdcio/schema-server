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

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/sdcio/schema-server/pkg/config"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

// sensitiveSchema loads the YANG fixture under testdata/sensitive and returns
// the resulting Schema for use in sub-tests.
func sensitiveSchema(t *testing.T) *Schema {
	t.Helper()
	sc, err := NewSchema(&config.SchemaConfig{
		Name:    "sensitive-test",
		Vendor:  "test",
		Version: "0.0.1",
		Files: []string{
			"testdata/sensitive",
		},
	})
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return sc
}

// TestIsSensitiveEntry verifies the low-level helper against hand-crafted
// yang.Entry objects — no YANG parsing required.
func TestIsSensitiveEntry(t *testing.T) {
	tests := []struct {
		name string
		exts []*yang.Statement
		want bool
	}{
		{
			name: "nil exts → not sensitive",
			exts: nil,
			want: false,
		},
		{
			name: "empty exts → not sensitive",
			exts: []*yang.Statement{},
			want: false,
		},
		{
			name: "unrelated extension → not sensitive",
			exts: []*yang.Statement{{Keyword: "other:tag"}},
			want: false,
		},
		{
			name: "sdcio-ext:sensitive → sensitive",
			exts: []*yang.Statement{{Keyword: "sdcio-ext:sensitive"}},
			want: true,
		},
		{
			name: "mixed exts including sdcio-ext:sensitive → sensitive",
			exts: []*yang.Statement{
				{Keyword: "other:tag"},
				{Keyword: "sdcio-ext:sensitive"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &yang.Entry{Exts: tt.exts}
			if got := isSensitiveEntry(e); got != tt.want {
				t.Errorf("isSensitiveEntry() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLeafSchema_SensitiveFlag loads the YANG fixture and verifies that
// LeafSchema.Sensitive is set correctly for leaves with and without the
// sdcio-ext:sensitive extension.
func TestLeafSchema_SensitiveFlag(t *testing.T) {
	sc := sensitiveSchema(t)

	tests := []struct {
		path      []string
		wantSens  bool
		wantField string
	}{
		{path: []string{"auth-password"}, wantSens: true, wantField: "auth-password (direct annotation)"},
		{path: []string{"description"}, wantSens: false, wantField: "description (no annotation)"},
		{path: []string{"plain-secret"}, wantSens: true, wantField: "plain-secret (deviate add in overlay)"},
	}

	for _, tt := range tests {
		t.Run(tt.wantField, func(t *testing.T) {
			e, err := sc.GetEntry(tt.path)
			if err != nil {
				t.Fatalf("GetEntry(%v): %v", tt.path, err)
			}
			elem, err := sc.SchemaElemFromYEntry(e, false)
			if err != nil {
				t.Fatalf("SchemaElemFromYEntry: %v", err)
			}
			leaf, ok := elem.Schema.(*sdcpb.SchemaElem_Field)
			if !ok {
				t.Fatalf("expected LeafSchema, got %T", elem.Schema)
			}
			if got := leaf.Field.GetSensitive(); got != tt.wantSens {
				t.Errorf("LeafSchema.Sensitive = %v, want %v", got, tt.wantSens)
			}
		})
	}
}

// TestLeafListSchema_SensitiveFlag verifies that LeafListSchema.Sensitive is
// set for leaf-lists annotated with sdcio-ext:sensitive.
func TestLeafListSchema_SensitiveFlag(t *testing.T) {
	sc := sensitiveSchema(t)

	e, err := sc.GetEntry([]string{"allowed-keys"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	elem, err := sc.SchemaElemFromYEntry(e, false)
	if err != nil {
		t.Fatalf("SchemaElemFromYEntry: %v", err)
	}
	ll, ok := elem.Schema.(*sdcpb.SchemaElem_Leaflist)
	if !ok {
		t.Fatalf("expected LeafListSchema, got %T", elem.Schema)
	}
	if !ll.Leaflist.GetSensitive() {
		t.Error("LeafListSchema.Sensitive = false, want true")
	}
}
