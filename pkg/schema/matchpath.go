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
)

type pathResolution struct {
	module *yang.Entry
	entry  *yang.Entry
}

func moduleHintMatches(mod *yang.Entry, prefix string) bool {
	if prefix == "" {
		return true
	}
	if mod == nil {
		return false
	}
	if mod.Name == prefix {
		return true
	}
	if mod.Prefix != nil && mod.Prefix.Name == prefix {
		return true
	}
	return false
}

func (sc *Schema) resolvePath(pe []string, origin string) ([]pathResolution, []string, error) {
	if len(pe) == 0 {
		return []pathResolution{{module: sc.root, entry: sc.root}}, nil, nil
	}

	parsed := ParsePathElems(pe, origin)
	names := UnprefixedNames(parsed)
	firstElem := pe[0]
	if parsed[0].Module != "" {
		firstElem = parsed[0].Module + ":" + parsed[0].Name
	}

	mods, err := sc.FindPossibleModulesForPathElement(sc.root, firstElem)
	if err != nil {
		return nil, names, err
	}

	winners := make([]pathResolution, 0, len(mods))
	moduleNames := make([]string, 0, len(mods))
	for _, mod := range mods {
		entry, err := getEntry(mod, names)
		if err != nil {
			continue
		}
		winners = append(winners, pathResolution{module: mod, entry: entry})
		moduleNames = append(moduleNames, mod.Name)
	}

	// Module introspection: single candidate module whose name equals the sole path element.
	if len(winners) == 0 && len(mods) == 1 && len(names) == 1 && mods[0].Name == names[0] {
		winners = []pathResolution{{module: mods[0], entry: mods[0]}}
		moduleNames = []string{mods[0].Name}
	}

	if len(winners) == 0 {
		return nil, names, fmt.Errorf("schema entry %q not found", strings.Join(names, "/"))
	}
	if len(winners) > 1 {
		sort.Slice(moduleNames, func(i, j int) bool {
			return utils.SortModulesAB(moduleNames[i], moduleNames[j], config.DeprioritizedModules)
		})
		prefix := strings.Join(names, "/")
		return nil, names, &AmbiguousPathError{PathPrefix: prefix, Modules: moduleNames}
	}
	return winners, names, nil
}

func (sc *Schema) GetEntryWithOrigin(pe []string, origin string) (*yang.Entry, error) {
	if len(pe) == 0 {
		sc.m.RLock()
		defer sc.m.RUnlock()
		return sc.root, nil
	}
	sc.m.RLock()
	defer sc.m.RUnlock()
	winners, _, err := sc.resolvePath(pe, origin)
	if err != nil {
		return nil, err
	}
	return winners[0].entry, nil
}
