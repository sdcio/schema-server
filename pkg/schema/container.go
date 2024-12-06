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
	"cmp"
	"slices"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (sc *Schema) containerFromYEntry(e *yang.Entry, withDesc bool) (*sdcpb.ContainerSchema, error) {
	c := &sdcpb.ContainerSchema{
		Name:              e.Name,
		Namespace:         e.Namespace().Name,
		Keys:              []*sdcpb.LeafSchema{},
		Fields:            []*sdcpb.LeafSchema{},
		Leaflists:         []*sdcpb.LeafListSchema{},
		Children:          []string{},
		MandatoryChildren: getMandatoryChildren(e),
		MustStatements:    getMustStatement(e),
		IsState:           isState(e),
		IsPresence:        isPresence(e),
		ChoiceInfo:        getChoiceInfo(e),
		IfFeature:         getIfFeature(e),
	}
	if withDesc {
		c.Description = e.Description
	}
	if e.ListAttr != nil {
		c.MaxElements = e.ListAttr.MaxElements
		c.MinElements = e.ListAttr.MinElements
		if e.ListAttr.OrderedBy != nil {
			c.IsUserOrdered = e.ListAttr.OrderedBy.Name == "user"
		}
	}
	if e.Prefix != nil {
		c.Prefix = e.Prefix.Name
		mod := yang.FindModuleByPrefix(e.Node, e.Prefix.Name)
		if mod != nil {
			c.ModuleName = mod.Name
		}
	}

	keys := strings.Fields(e.Key)
	for _, child := range getChildren(e) {
		switch {
		case child.IsDir():
			c.Children = append(c.Children, child.Name)
		case child.IsLeaf(), child.IsLeafList():
			o, err := sc.SchemaElemFromYEntry(child, withDesc)
			if err != nil {
				return nil, err
			}
			switch o := o.Schema.(type) {
			case *sdcpb.SchemaElem_Field:
				if slices.Index(keys, child.Name) != -1 {
					c.Keys = append(c.Keys, o.Field)
					continue
				}
				c.Fields = append(c.Fields, o.Field)
			case *sdcpb.SchemaElem_Leaflist:
				c.Leaflists = append(c.Leaflists, o.Leaflist)
			}
		}
	}
	slices.Sort(c.Children)
	// keep ordering based on schema
	slices.SortFunc(c.Keys, func(a, b *sdcpb.LeafSchema) int {
		return cmp.Compare(slices.Index(keys, a.GetName()), slices.Index(keys, b.GetName()))
	})
	return c, nil
}

func getMandatoryChildren(e *yang.Entry) []*sdcpb.MandatoryChild {
	result := []*sdcpb.MandatoryChild{}
	for k, v := range e.Dir {
		if v.Mandatory == yang.TSTrue {
			c := &sdcpb.MandatoryChild{
				Name:    k,
				IsState: isState(v),
			}
			result = append(result, c)
		}
	}
	return result
}

func isState(e *yang.Entry) bool {
	if e.Config == yang.TSFalse {
		return true
	}
	if e.Parent != nil {
		return isState(e.Parent)
	}
	return false
}

func isPresence(e *yang.Entry) bool {
	if presence, ok := e.Extra["presence"]; ok {
		for _, m := range presence {
			if _, ok := m.(*yang.Value); ok {
				return ok
			}
		}
		return false
	}
	return false
}

func getChoiceInfo(e *yang.Entry) *sdcpb.ChoiceInfo {
	if e == nil {
		return nil
	}

	var ci *sdcpb.ChoiceInfo

	// go through child entries
	for _, de := range e.Dir {
		// continue if it is not a choice
		if !de.IsChoice() {
			continue
		}

		// init the return struct if still nil
		if ci == nil {
			// create ChoiceInfo return struct
			ci = &sdcpb.ChoiceInfo{
				Choice: map[string]*sdcpb.ChoiceInfoChoice{},
			}
		}

		processChoice(de, ci)
	}
	return ci
}

func processChoice(e *yang.Entry, ci *sdcpb.ChoiceInfo) {
	ci.Choice[e.Name] = &sdcpb.ChoiceInfoChoice{
		Case: map[string]*sdcpb.ChoiceCase{},
	}

	for childName, cases := range e.Dir {
		actualCase := &sdcpb.ChoiceCase{}
		ci.Choice[e.Name].Case[childName] = actualCase

		for caseElementName := range cases.Dir {
			actualCase.Elements = append(actualCase.Elements, caseElementName)
		}
	}
}
