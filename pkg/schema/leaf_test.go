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

	"github.com/sdcio/schema-server/pkg/config"
)

func mustAliasSchema(t *testing.T) *Schema {
	t.Helper()
	sc, err := NewSchema(&config.SchemaConfig{
		Name:        "must-alias-test",
		Vendor:      "test",
		Version:     "0.0.0",
		Files:       []string{"testdata/must-alias"},
		Directories: []string{},
		Excludes:    []string{},
	})
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return sc
}

// TestGetMustStatement_NormalizesAlias verifies that getMustStatement rewrites
// quoted xpath literals that use a non-standard import alias to the declared
// prefix of the imported module.
func TestGetMustStatement_NormalizesAlias(t *testing.T) {
	sc := mustAliasSchema(t)

	e, err := sc.GetEntry([]string{"protocol-with-alias"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	stmts := getMustStatement(e)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 must statement, got %d", len(stmts))
	}

	got := stmts[0].Statement
	want := ". = 'must-id:BGP' or . = 'must-id:OSPF'"
	if got != want {
		t.Errorf("must statement not normalized\n got:  %q\n want: %q", got, want)
	}
}

// TestGetMustStatement_AlreadyDeclaredPrefix verifies that getMustStatement
// leaves literals unchanged when they already use the module's declared prefix
// (idempotent behaviour).
func TestGetMustStatement_AlreadyDeclaredPrefix(t *testing.T) {
	sc := mustAliasSchema(t)

	e, err := sc.GetEntry([]string{"protocol-correct-prefix"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	stmts := getMustStatement(e)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 must statement, got %d", len(stmts))
	}

	got := stmts[0].Statement
	want := ". = 'must-id:BGP'"
	if got != want {
		t.Errorf("must statement changed unexpectedly\n got:  %q\n want: %q", got, want)
	}
}

// TestGetMustStatement_PlainLiteralUnchanged verifies that string literals
// without a colon prefix are left unchanged.
func TestGetMustStatement_PlainLiteralUnchanged(t *testing.T) {
	sc := mustAliasSchema(t)

	e, err := sc.GetEntry([]string{"plain-string-must"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	stmts := getMustStatement(e)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 must statement, got %d", len(stmts))
	}

	got := stmts[0].Statement
	want := ". != 'just-a-string'"
	if got != want {
		t.Errorf("plain literal changed unexpectedly\n got:  %q\n want: %q", got, want)
	}
}

// TestGetMustStatement_UnknownPrefixUnchanged verifies that literals whose
// prefix is not in the module's import table are left unchanged.
func TestGetMustStatement_UnknownPrefixUnchanged(t *testing.T) {
	sc := mustAliasSchema(t)

	e, err := sc.GetEntry([]string{"unknown-prefix-must"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}

	stmts := getMustStatement(e)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 must statement, got %d", len(stmts))
	}

	got := stmts[0].Statement
	want := ". != 'unknown-mod:FOO'"
	if got != want {
		t.Errorf("unknown-prefix literal changed unexpectedly\n got:  %q\n want: %q", got, want)
	}
}
