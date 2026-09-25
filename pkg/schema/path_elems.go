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

import "strings"

// PathElem is a parsed path element with an optional module hint (module prefix or gNMI origin).
type PathElem struct {
	Name   string // unprefixed element name
	Module string // optional module name when prefix or origin is set
}

// ParsePathElems splits optional "module:name" prefixes on each path element.
// When origin is non-empty and the first element has no prefix, origin is applied as the first element's module hint.
func ParsePathElems(pes []string, origin string) []PathElem {
	out := make([]PathElem, 0, len(pes))
	for _, pe := range pes {
		if i := strings.IndexByte(pe, ':'); i > 0 {
			out = append(out, PathElem{
				Module: pe[:i],
				Name:   pe[i+1:],
			})
		} else {
			out = append(out, PathElem{Name: pe})
		}
	}
	if origin != "" && len(out) > 0 && out[0].Module == "" {
		out[0].Module = origin
	}
	return out
}

// UnprefixedNames returns the local names for each parsed path element.
func UnprefixedNames(pes []PathElem) []string {
	names := make([]string, len(pes))
	for i, pe := range pes {
		names[i] = pe.Name
	}
	return names
}
