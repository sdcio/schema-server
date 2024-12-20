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

package utils

import "testing"

func TestSortModulesAB(t *testing.T) {
	type args struct {
		a                    string
		b                    string
		deprioritizedModules []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "A before B",
			args: args{
				a:                    "A",
				b:                    "B",
				deprioritizedModules: []string{"ietf"},
			},
			want: true,
		},
		{
			name: "ABCDE before B",
			args: args{
				a:                    "ABCDE",
				b:                    "B",
				deprioritizedModules: []string{"ietf"},
			},
			want: true,
		},
		{
			name: "ietf_interface beforeafter XModule",
			args: args{
				a:                    "ietf_interface",
				b:                    "XModule",
				deprioritizedModules: []string{"ietf"},
			},
			want: false,
		},
		{
			name: "AModule before XModule",
			args: args{
				a:                    "AModule",
				b:                    "XModule",
				deprioritizedModules: []string{"ietf"},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SortModulesAB(tt.args.a, tt.args.b, tt.args.deprioritizedModules); got != tt.want {
				t.Errorf("SortModulesAB() = %v, want %v", got, tt.want)
			}
		})
	}
}
