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
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/utils"
)

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
