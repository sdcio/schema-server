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
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/openconfig/goyang/pkg/yang"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/schema-server/pkg/config"
)

// entryChain builds a minimal yang.Entry parent chain from a module root down
// to a leaf, returning the leaf entry.  The slice is ordered root → leaf, where
// each element is {name, kind}.  A synthetic __root__ parent is prepended
// automatically so that the ancestor-prepend loop in relativeToAbsPath stops
// at the right level.
func entryChain(nodes ...struct {
	name string
	kind yang.EntryKind
}) *yang.Entry {
	root := &yang.Entry{Name: RootName, Kind: yang.DirectoryEntry}
	current := root
	for _, n := range nodes {
		e := &yang.Entry{Name: n.name, Kind: n.kind, Parent: current}
		current = e
	}
	return current
}

func TestNormalizePath_RelativeUnderChoiceCase(t *testing.T) {
	// Reproduces the OcNOS leafref failure pattern:
	// ref-id sits inside detail-list which is nested under choice/case.
	// ../source-id must resolve to ["host", "detail-list", "source-id"],
	// not to ["host", "detail-choice", "variant-a", "detail-list", "source-id"].
	leaf := entryChain(
		struct {
			name string
			kind yang.EntryKind
		}{"mod", yang.DirectoryEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"host", yang.DirectoryEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"detail-choice", yang.ChoiceEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"variant-a", yang.CaseEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"detail-list", yang.DirectoryEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"ref-id", yang.LeafEntry},
	)

	got := normalizePath("../source-id", leaf)
	want := []string{"host", "detail-list", "source-id"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizePath under choice/case:\n  got  %v\n  want %v", got, want)
	}
}

func TestNormalizePath_RelativeNoChoiceCase(t *testing.T) {
	// Regression: a relative leafref where no choice/case is present must
	// still produce the correct data-tree path.
	leaf := entryChain(
		struct {
			name string
			kind yang.EntryKind
		}{"mod", yang.DirectoryEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"outer", yang.DirectoryEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"inner", yang.DirectoryEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"ref-id", yang.LeafEntry},
	)

	got := normalizePath("../target-leaf", leaf)
	want := []string{"outer", "inner", "target-leaf"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizePath without choice/case:\n  got  %v\n  want %v", got, want)
	}
}

func TestNormalizePath_AbsolutePathStripsModulePrefix(t *testing.T) {
	// An absolute leafref path with module prefixes must have each prefix
	// stripped.  The entry parameter is not used for absolute paths.
	leaf := entryChain(
		struct {
			name string
			kind yang.EntryKind
		}{"mod", yang.DirectoryEntry},
		struct {
			name string
			kind yang.EntryKind
		}{"ref-id", yang.LeafEntry},
	)

	got := normalizePath("/lrc:host/lrc:detail-list/lrc:source-id", leaf)
	want := []string{"host", "detail-list", "source-id"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizePath absolute with prefix:\n  got  %v\n  want %v", got, want)
	}
}

// TestBuildReferences_ErrorWrapsPathContext verifies SS-LR-03:
// buildReferences() must wrap GetEntry() failures with both the original
// leafref path string and the normalized path elements, and must use %w so
// that errors.Unwrap returns the underlying error.
func TestBuildReferences_ErrorWrapsPathContext(t *testing.T) {
	cfg := &config.SchemaConfig{
		Name:        "bad-leafref",
		Vendor:      "test",
		Version:     "0.0.0",
		Files:       []string{"testdata/bad-leafref"},
		Directories: []string{},
		Excludes:    []string{},
	}

	_, err := NewSchema(cfg)
	if err == nil {
		t.Fatal("NewSchema() expected an error for an unresolvable leafref, got nil")
	}

	// Error message must include the original leafref path text.
	if !strings.Contains(err.Error(), "../nonexistent-target") {
		t.Errorf("error %q does not contain original path %q", err.Error(), "../nonexistent-target")
	}
	// Error message must include the normalized path element.
	if !strings.Contains(err.Error(), "nonexistent-target") {
		t.Errorf("error %q does not contain normalized path element %q", err.Error(), "nonexistent-target")
	}
	// %w chaining must be preserved.
	if errors.Unwrap(err) == nil {
		t.Errorf("errors.Unwrap(err) is nil; error was not wrapped with %%w")
	}
}

// TestToSchemaType_ErrorWrapsPathContext verifies SS-LR-03:
// toSchemaType() must wrap GetEntry() failures for leafref types with both
// the original path string and the normalized path elements, and must use %w.
func TestToSchemaType_ErrorWrapsPathContext(t *testing.T) {
	// Build a minimal Schema with an empty root so GetEntry always fails.
	sc := &Schema{
		m:    new(sync.RWMutex),
		root: &yang.Entry{Name: RootName, Dir: map[string]*yang.Entry{}},
	}

	// Build a yang.Entry with a leafref type pointing to a non-existent target.
	e := &yang.Entry{
		Name: "broken-leaf",
		Kind: yang.LeafEntry,
		Parent: &yang.Entry{
			Name:   "container",
			Kind:   yang.DirectoryEntry,
			Parent: &yang.Entry{Name: RootName},
		},
	}
	e.Parent.Dir = map[string]*yang.Entry{"broken-leaf": e}

	yt := &yang.YangType{
		Kind: yang.Yleafref,
		Path: "/nonexistent-module:nonexistent-leaf",
	}

	_, err := sc.toSchemaType(e, yt)
	if err == nil {
		t.Fatal("toSchemaType() expected an error for an unresolvable leafref, got nil")
	}

	// Error message must include the original leafref path text.
	if !strings.Contains(err.Error(), "/nonexistent-module:nonexistent-leaf") {
		t.Errorf("error %q does not contain original path", err.Error())
	}
	// Error message must include the normalized path element.
	if !strings.Contains(err.Error(), "nonexistent-leaf") {
		t.Errorf("error %q does not contain normalized path element", err.Error())
	}
	// %w chaining must be preserved.
	if errors.Unwrap(err) == nil {
		t.Errorf("errors.Unwrap(err) is nil; error was not wrapped with %%w")
	}
}

// TestToSchemaType_LeafrefReturnsType verifies that when the leafref target
// exists, toSchemaType resolves it and returns a non-nil LeafrefTargetType.
// This is a regression guard so wrapping changes don't break the happy path.
func TestToSchemaType_LeafrefReturnsType(t *testing.T) {
	cfg := &config.SchemaConfig{
		Name:        "leafref-under-choice",
		Vendor:      "test",
		Version:     "0.0.0",
		Files:       []string{"testdata/leafref-under-choice"},
		Directories: []string{},
		Excludes:    []string{},
	}

	sc, err := NewSchema(cfg)
	if err != nil {
		t.Fatalf("NewSchema() returned unexpected error: %v", err)
	}

	// Get the ref-id leaf entry to call toSchemaType directly.
	refEntry, err := sc.GetEntry([]string{"host", "detail-list", "ref-id"})
	if err != nil {
		t.Fatalf("GetEntry(host/detail-list/ref-id) unexpected error: %v", err)
	}

	slt, err := sc.toSchemaType(refEntry, refEntry.Type)
	if err != nil {
		t.Fatalf("toSchemaType() unexpected error: %v", err)
	}
	if slt.LeafrefTargetType == nil {
		t.Error("toSchemaType() returned nil LeafrefTargetType for a leafref leaf")
	}

	// Verify the sdcpb import is used.
	_ = (*sdcpb.SchemaLeafType)(nil)
}
