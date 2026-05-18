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

package persiststore

import (
	"reflect"
	"testing"
)

func TestParsePathElems(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []string
		want []pathElem
	}{
		// nil input yields empty slice (not nil) from make(…, 0, len(pes))
		{nil, []pathElem{}},
		{[]string{}, []pathElem{}},
		{[]string{"a"}, []pathElem{{name: "a"}}},
		{[]string{"m:a"}, []pathElem{{module: "m", name: "a"}}},
		{[]string{"foo:bar:baz"}, []pathElem{{module: "foo", name: "bar:baz"}}},
		{[]string{":edge"}, []pathElem{{name: ":edge"}}},
		{[]string{"no-colon-here"}, []pathElem{{name: "no-colon-here"}}},
		{
			[]string{"aug-base:target", "aug-extra:augment-only-leaf"},
			[]pathElem{
				{module: "aug-base", name: "target"},
				{module: "aug-extra", name: "augment-only-leaf"},
			},
		},
	}
	for _, tc := range cases {
		got := parsePathElems(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("parsePathElems(%q) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}
