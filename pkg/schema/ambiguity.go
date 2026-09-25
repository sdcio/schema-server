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
	"fmt"
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/utils"
	log "github.com/sirupsen/logrus"
)

// duplicateNodeErrorMarker matches the "Duplicate node ..." error goyang's Entry.merge() records
// (via addError) whenever two augments, or an augment and the base module, introduce a child with
// the same local name at the same target. Since GY-1 (sdcio/goyang#5), the losing child is
// retained in Entry.Collisions instead of being silently dropped -- but the error itself is the
// only load-time signal that a collision happened at all.
const duplicateNodeErrorMarker = "Duplicate node"

// reportAugmentMergeCollisions inspects every top-level module entry's GetErrors() (which
// recurses into Dir) for goyang's "Duplicate node" augment-merge-collision error and:
//   - logs each one loudly (schema-load-time, not silent) so operators know a collision occurred
//     and was resolved via the nested ambiguity registry rather than a lexicographic pick, and
//   - fails schema load if a collision was reported but has no corresponding recoverable entry in
//     yang.Entry.Collisions -- i.e. genuine, unrecoverable data loss that GY-1 was supposed to
//     prevent. This should not happen given GY-1's merge() fix; treat it as a hard integration
//     bug (mismatched goyang version, or a merge path GY-1 didn't cover) rather than continuing
//     silently as schema-server did before ticket 10.
func reportAugmentMergeCollisions(root *yang.Entry, schemaName string) error {
	if root == nil {
		return nil
	}
	var unrecovered []string
	for _, mod := range root.Dir {
		if mod == nil {
			continue
		}
		for _, err := range mod.GetErrors() {
			if !strings.Contains(err.Error(), duplicateNodeErrorMarker) {
				continue
			}
			log.Warnf("schema %s: augment-merge collision detected at load (recovered via nested ambiguity registry, module-qualify to disambiguate): %v", schemaName, err)
		}
	}
	if err := checkCollisionsRecovered(root, &unrecovered); err != nil {
		return err
	}
	if len(unrecovered) > 0 {
		return fmt.Errorf("schema %s: %d augment-merge collision(s) reported by goyang with no recoverable entry in yang.Entry.Collisions (unrecovered data loss): %s",
			schemaName, len(unrecovered), strings.Join(unrecovered, "; "))
	}
	return nil
}

// checkCollisionsRecovered walks the tree looking for any entry that recorded a direct
// "Duplicate node" error (Entry.Errors, not the recursive GetErrors()) without a matching
// Entry.Collisions entry -- the integrity check described on reportAugmentMergeCollisions.
func checkCollisionsRecovered(e *yang.Entry, unrecovered *[]string) error {
	if e == nil {
		return nil
	}
	for _, err := range e.Errors {
		if !strings.Contains(err.Error(), duplicateNodeErrorMarker) {
			continue
		}
		if len(e.Collisions) == 0 {
			*unrecovered = append(*unrecovered, err.Error())
		}
	}
	for _, child := range e.Dir {
		if err := checkCollisionsRecovered(child, unrecovered); err != nil {
			return err
		}
	}
	return nil
}

// RootNameAmbiguity describes a local name that appears under more than one top-level module.
type RootNameAmbiguity struct {
	LocalName string
	Modules   []string
}

func buildRootAmbiguityRegistry(root *yang.Entry) []RootNameAmbiguity {
	if root == nil || root.Dir == nil {
		return nil
	}
	localToMods := make(map[string][]string)
	for modName, mod := range root.Dir {
		if mod == nil || mod.Dir == nil {
			continue
		}
		for childName := range mod.Dir {
			localToMods[childName] = append(localToMods[childName], modName)
		}
	}
	out := make([]RootNameAmbiguity, 0)
	for local, mods := range localToMods {
		if len(mods) < 2 {
			continue
		}
		sort.Slice(mods, func(i, j int) bool {
			return utils.SortModulesAB(mods[i], mods[j], config.DeprioritizedModules)
		})
		out = append(out, RootNameAmbiguity{LocalName: local, Modules: mods})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LocalName < out[j].LocalName
	})
	return out
}

// RootAmbiguities returns root-level colliding local names discovered at schema load time.
func (sc *Schema) RootAmbiguities() []RootNameAmbiguity {
	if sc == nil {
		return nil
	}
	sc.m.RLock()
	defer sc.m.RUnlock()
	return sc.rootAmbiguities
}

// AmbiguityRegistryExcludeEntries encodes root ambiguities for GetSchemaDetailsResponse.exclude.
func AmbiguityRegistryExcludeEntries(ambiguities []RootNameAmbiguity) []string {
	if len(ambiguities) == 0 {
		return nil
	}
	out := make([]string, 0, len(ambiguities))
	for _, a := range ambiguities {
		out = append(out, AmbiguousNameRegistryExcludePrefix+"/"+a.LocalName+"="+strings.Join(a.Modules, ","))
	}
	return out
}

// NestedNameAmbiguity describes a local name that collided during augment-merge at a non-root
// position -- i.e. two cross-module augments, or an augment and the base module, introducing a
// child with the same local name under the same parent (e.g. `ethernet` under
// `/interfaces/interface`, `pm`/`fec`/`threshold` under `/controllers/controller-preconfigure` in
// Cisco IOS-XR's um-* YANG set). See ticket 10 in sdcio/data-server's
// .scratch/cisco-ios-xr-schema-collision for the fuller design discussion (SS-2).
//
// This is the non-root counterpart of RootNameAmbiguity: root ambiguities are same-named
// top-level containers declared by different modules; nested ambiguities are same-named children
// produced by goyang's augment merge() at any depth, recoverable via yang.Entry.Collisions/
// Candidates since GY-1 (sdcio/goyang#5) stopped dropping the losing child.
type NestedNameAmbiguity struct {
	ParentPath []string // schema path (local names, root-relative, no module qualifiers) to the parent of the colliding children
	LocalName  string
	Modules    []string // owning module of every candidate (Dir winner + Collisions losers), sorted
}

// buildNestedAmbiguityRegistry walks the compiled schema tree below each top-level module and
// records every position where yang.Entry.Collisions retained one or more losing augment-merge
// children alongside the winner reachable via Dir[name]. Unlike buildRootAmbiguityRegistry
// (root Dir only), this descends the whole tree: ticket 10's confirmed collisions are not
// confined to the root (nested `/controllers/...`, `/interfaces/...` targets).
func buildNestedAmbiguityRegistry(root *yang.Entry) []NestedNameAmbiguity {
	if root == nil || root.Dir == nil {
		return nil
	}
	out := make([]NestedNameAmbiguity, 0)
	seen := make(map[string]bool)

	var walk func(e *yang.Entry, path []string)
	walk = func(e *yang.Entry, path []string) {
		if e == nil {
			return
		}
		for name := range e.Collisions {
			key := strings.Join(path, "/") + "\x00" + name
			if seen[key] {
				continue
			}
			seen[key] = true

			candidates := e.Candidates(name)
			modSet := make(map[string]bool, len(candidates))
			mods := make([]string, 0, len(candidates))
			for _, c := range candidates {
				m := ownerModule(c)
				if m == "" || modSet[m] {
					continue
				}
				modSet[m] = true
				mods = append(mods, m)
			}
			sort.Slice(mods, func(i, j int) bool {
				return utils.SortModulesAB(mods[i], mods[j], config.DeprioritizedModules)
			})
			out = append(out, NestedNameAmbiguity{
				ParentPath: append([]string{}, path...),
				LocalName:  name,
				Modules:    mods,
			})
		}
		for childName, child := range e.Dir {
			walk(child, append(append([]string{}, path...), childName))
		}
	}
	// Start below each top-level module: the module itself is a namespace, not a data-tree
	// path element, so its own Dir children start at the root-relative path "".
	for _, mod := range root.Dir {
		walk(mod, []string{})
	}

	sort.Slice(out, func(i, j int) bool {
		pi, pj := strings.Join(out[i].ParentPath, "/"), strings.Join(out[j].ParentPath, "/")
		if pi != pj {
			return pi < pj
		}
		return out[i].LocalName < out[j].LocalName
	})
	return out
}

// NestedAmbiguities returns non-root colliding local names (augment-merge collisions, ticket 10)
// discovered at schema load time.
func (sc *Schema) NestedAmbiguities() []NestedNameAmbiguity {
	if sc == nil {
		return nil
	}
	sc.m.RLock()
	defer sc.m.RUnlock()
	return sc.nestedAmbiguities
}

// NestedAmbiguityRegistryExcludeEntries encodes nested (non-root) ambiguities for
// GetSchemaDetailsResponse.exclude, using the same AmbiguousNameRegistryExcludePrefix as root
// ambiguities but with the full parent path before the local name, e.g.
// "sdcio.schema/ambiguous-name:/interfaces/interface/ethernet=modA,modB".
func NestedAmbiguityRegistryExcludeEntries(ambiguities []NestedNameAmbiguity) []string {
	if len(ambiguities) == 0 {
		return nil
	}
	out := make([]string, 0, len(ambiguities))
	for _, a := range ambiguities {
		segs := append(append([]string{}, a.ParentPath...), a.LocalName)
		out = append(out, AmbiguousNameRegistryExcludePrefix+"/"+strings.Join(segs, "/")+"="+strings.Join(a.Modules, ","))
	}
	return out
}
