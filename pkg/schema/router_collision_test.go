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
	"strings"
	"testing"

	"github.com/sdcio/schema-server/pkg/config"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func loadRouterCollisionSchema(t *testing.T) *Schema {
	t.Helper()
	cfg := &config.SchemaConfig{
		Name:        "router-collision",
		Vendor:      "cisco",
		Version:     "1",
		Files:       []string{"testdata/router-collision"},
		Directories: []string{},
	}
	sc, err := NewSchema(cfg)
	if err != nil {
		t.Fatalf("NewSchema: %v", err)
	}
	return sc
}

func TestRouterCollision_DeepPathResolves(t *testing.T) {
	sc := loadRouterCollisionSchema(t)
	e, err := sc.GetEntry([]string{"router", "isis", "enabled"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if e.Name != "enabled" {
		t.Fatalf("expected enabled leaf, got %q", e.Name)
	}
}

func TestRouterCollision_ShallowPathAmbiguous(t *testing.T) {
	sc := loadRouterCollisionSchema(t)
	_, err := sc.GetEntry([]string{"router"})
	if err == nil {
		t.Fatal("expected ambiguous error for bare router")
	}
	var amb *AmbiguousPathError
	if !errors.As(err, &amb) {
		t.Fatalf("expected AmbiguousPathError, got %v", err)
	}
	if len(amb.Modules) != 3 {
		t.Fatalf("expected 3 modules, got %v", amb.Modules)
	}
}

func TestRouterCollision_PrefixedPath(t *testing.T) {
	sc := loadRouterCollisionSchema(t)
	e, err := sc.GetEntry([]string{"cisco-router-ospf:router", "ospf", "enabled"})
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if e.Name != "enabled" {
		t.Fatalf("expected enabled leaf, got %q", e.Name)
	}
}

func TestRouterCollision_BuildPathAmbiguous(t *testing.T) {
	sc := loadRouterCollisionSchema(t)
	p := &sdcpb.Path{}
	err := sc.BuildPath([]string{"router"}, p)
	var amb *AmbiguousPathError
	if !errors.As(err, &amb) {
		t.Fatalf("expected AmbiguousPathError, got %v", err)
	}
}

func TestRouterCollision_BuildPathDeep(t *testing.T) {
	sc := loadRouterCollisionSchema(t)
	p := &sdcpb.Path{}
	if err := sc.BuildPath([]string{"router", "static", "enabled"}, p); err != nil {
		t.Fatalf("BuildPath: %v", err)
	}
	if len(p.Elem) != 3 {
		t.Fatalf("expected 3 path elems, got %d", len(p.Elem))
	}
}

func TestRouterCollision_RootAmbiguityRegistry(t *testing.T) {
	sc := loadRouterCollisionSchema(t)
	amb := sc.RootAmbiguities()
	if len(amb) != 1 || amb[0].LocalName != "router" || len(amb[0].Modules) != 3 {
		t.Fatalf("unexpected ambiguities: %+v", amb)
	}
	entries := AmbiguityRegistryExcludeEntries(amb)
	if len(entries) != 1 || !strings.HasPrefix(entries[0], AmbiguousNameRegistryExcludePrefix) {
		t.Fatalf("unexpected registry entries: %v", entries)
	}
}
