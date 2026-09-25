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
	"sort"
	"testing"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"

	"github.com/sdcio/schema-server/pkg/config"
)

// loadAugmentCollisionSchema loads testdata/augment-collision, which reproduces the real Cisco
// IOS-XR pattern from ticket 10 (.scratch/cisco-ios-xr-schema-collision): two independent modules
// ("augment-one", "augment-two") each augment "base"'s "/root" with a container sharing the local
// name "clash". Since GY-1 (sdcio/goyang#5), goyang's merge() no longer drops the losing child --
// it survives in yang.Entry.Collisions -- so SS-2's job is to make both reachable/erroring
// correctly instead of silently returning whichever one merge() happened to keep in Dir.
func loadAugmentCollisionSchema(t *testing.T) *Schema {
	t.Helper()
	cfg := &config.SchemaConfig{
		Name:        "augment-collision",
		Vendor:      "cisco",
		Version:     "1",
		Files:       []string{"testdata/augment-collision"},
		Directories: []string{},
	}
	sc, err := NewSchema(cfg)
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return sc
}

func TestAugmentCollision_BareChildAmbiguous(t *testing.T) {
	sc := loadAugmentCollisionSchema(t)
	_, err := sc.GetEntry([]string{"root", "clash"})
	if err == nil {
		t.Fatal("expected ambiguous error for bare root/clash")
	}
	var amb *AmbiguousPathError
	if !errors.As(err, &amb) {
		t.Fatalf("expected AmbiguousPathError, got %v (%T)", err, err)
	}
	if len(amb.Modules) != 2 {
		t.Fatalf("expected 2 colliding modules, got %v", amb.Modules)
	}
	wantMods := []string{"augment-one", "augment-two"}
	gotMods := append([]string{}, amb.Modules...)
	sort.Strings(gotMods)
	sort.Strings(wantMods)
	if gotMods[0] != wantMods[0] || gotMods[1] != wantMods[1] {
		t.Fatalf("expected modules %v, got %v", wantMods, gotMods)
	}
}

func TestAugmentCollision_BuildPathBareChildAmbiguous(t *testing.T) {
	sc := loadAugmentCollisionSchema(t)
	p := &sdcpb.Path{}
	err := sc.BuildPath([]string{"root", "clash"}, p)
	var amb *AmbiguousPathError
	if !errors.As(err, &amb) {
		t.Fatalf("expected AmbiguousPathError, got %v (%T)", err, err)
	}
}

func TestAugmentCollision_MarkerLeafUnaffected(t *testing.T) {
	// A non-colliding sibling under the same parent must resolve normally -- the collision
	// guard must not affect ordinary, unambiguous children.
	sc := loadAugmentCollisionSchema(t)
	e, err := sc.GetEntry([]string{"root", "marker"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if e.Name != "marker" {
		t.Fatalf("expected marker leaf, got %q", e.Name)
	}
}

func TestAugmentCollision_NestedAmbiguityRegistry(t *testing.T) {
	sc := loadAugmentCollisionSchema(t)
	nested := sc.NestedAmbiguities()
	if len(nested) != 1 {
		t.Fatalf("expected 1 nested ambiguity, got %d: %+v", len(nested), nested)
	}
	a := nested[0]
	if a.LocalName != "clash" {
		t.Fatalf("expected local name %q, got %q", "clash", a.LocalName)
	}
	if len(a.ParentPath) != 1 || a.ParentPath[0] != "root" {
		t.Fatalf("expected parent path [\"root\"], got %v", a.ParentPath)
	}
	if len(a.Modules) != 2 {
		t.Fatalf("expected 2 modules, got %v", a.Modules)
	}

	entries := NestedAmbiguityRegistryExcludeEntries(nested)
	if len(entries) != 1 {
		t.Fatalf("expected 1 registry entry, got %v", entries)
	}
	want := AmbiguousNameRegistryExcludePrefix + "/root/clash=augment-one,augment-two"
	if entries[0] != want {
		t.Fatalf("expected registry entry %q, got %q", want, entries[0])
	}
}

func TestLookupChild_AmbiguousWithoutPrefix(t *testing.T) {
	sc := loadAugmentCollisionSchema(t)
	root, err := sc.GetEntry([]string{"root"})
	if err != nil {
		t.Fatalf("GetEntry(root): %v", err)
	}
	_, err = lookupChild(root, "clash")
	var amb *AmbiguousPathError
	if !errors.As(err, &amb) {
		t.Fatalf("expected AmbiguousPathError, got %v (%T)", err, err)
	}
}

func TestLookupChild_ResolvesWithModulePrefix(t *testing.T) {
	sc := loadAugmentCollisionSchema(t)
	root, err := sc.GetEntry([]string{"root"})
	if err != nil {
		t.Fatalf("GetEntry(root): %v", err)
	}
	for _, mod := range []string{"augment-one", "augment-two"} {
		child, err := lookupChild(root, mod+":clash")
		if err != nil {
			t.Fatalf("lookupChild(%s:clash): %v", mod, err)
		}
		if child == nil {
			t.Fatalf("lookupChild(%s:clash): expected a resolved entry, got nil", mod)
		}
		if ownerModule(child) != mod {
			t.Fatalf("lookupChild(%s:clash): expected owner module %q, got %q", mod, mod, ownerModule(child))
		}
	}
}
