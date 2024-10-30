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
	"github.com/openconfig/goyang/pkg/yang"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func leafListFromYEntry(e *yang.Entry, withDesc bool) *sdcpb.LeafListSchema {
	ll := &sdcpb.LeafListSchema{
		Name:           e.Name,
		Namespace:      e.Namespace().Name,
		Type:           toSchemaType(e.Type),
		Units:          e.Units,
		MustStatements: getMustStatement(e),
		IsState:        isState(e),
		IsUserOrdered:  false,
		IfFeature:      getIfFeature(e),
		Defaults:       e.DefaultValues(),
	}
	if withDesc {
		ll.Description = e.Description
	}
	if e.ListAttr != nil {
		ll.MaxElements = e.ListAttr.MaxElements
		ll.MinElements = e.ListAttr.MinElements
		if e.ListAttr.OrderedBy != nil {
			ll.IsUserOrdered = e.ListAttr.OrderedBy.Name == "user"
		}
	}

	if e.Prefix != nil {
		ll.Prefix = e.Prefix.Name
		mod := yang.FindModuleByPrefix(e.Node, e.Prefix.Name)
		if mod != nil {
			ll.ModuleName = mod.Name
		}
	}

	return ll
}
