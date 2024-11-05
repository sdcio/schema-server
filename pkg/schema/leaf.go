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
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/sdcio/schema-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
)

func (sc *Schema) leafFromYEntry(e *yang.Entry, withDesc bool) (*sdcpb.LeafSchema, error) {
	entryType, err := sc.toSchemaType(e, e.Type)
	if err != nil {
		return nil, err
	}

	l := &sdcpb.LeafSchema{
		Name:           e.Name,
		Namespace:      e.Namespace().Name,
		Type:           entryType,
		IsMandatory:    e.Mandatory.Value(),
		Units:          e.Units,
		MustStatements: getMustStatement(e),
		IsState:        isState(e),
		Reference:      make([]string, 0),
		IfFeature:      getIfFeature(e),
	}

	if withDesc {
		l.Description = e.Description
	}
	if v, ok := e.SingleDefaultValue(); ok {
		l.Default = v
	}
	if e.Prefix != nil {
		l.Prefix = e.Prefix.Name
		mod := yang.FindModuleByPrefix(e.Node, e.Prefix.Name)
		if mod != nil {
			l.ModuleName = mod.Name
		}
	}
	for k, v := range e.Annotation {
		// fmt.Println("annotation:", k)
		if !strings.HasPrefix(k, "REF_") {
			continue
		}
		switch v := v.(type) {
		case *yang.Entry:
			// l.Reference = append(l.Reference, v.Path())
			l.Reference = append(l.Reference, utils.ToXPath(buildPathUpFromEntry(v), true))
		}
	}

	return l, nil
}

// toSchemaType e is the yang.Entry that the type belongs to, yt is the actual type.
func (sc *Schema) toSchemaType(e *yang.Entry, yt *yang.YangType) (*sdcpb.SchemaLeafType, error) {
	var enumNames []string
	if yt.Enum != nil {
		enumNames = yt.Enum.Names()
	}

	slt := &sdcpb.SchemaLeafType{
		Type:       yt.Kind.String(),
		Range:      []*sdcpb.SchemaMinMaxType{},
		Length:     []*sdcpb.SchemaMinMaxType{},
		EnumNames:  enumNames,
		Units:      yt.Units,
		TypeName:   yt.Name,
		Leafref:    yt.Path,
		Patterns:   []*sdcpb.SchemaPattern{},
		UnionTypes: []*sdcpb.SchemaLeafType{},
	}

	for _, l := range yt.Length {
		slt.Length = append(slt.Length, yRangeToSchemaMinMaxType(&l))
	}

	for _, r := range yt.Range {
		slt.Range = append(slt.Range, yRangeToSchemaMinMaxType(&r))
	}

	for _, pat := range yt.Pattern {
		slt.Patterns = append(slt.Patterns, &sdcpb.SchemaPattern{
			Pattern:  pat,
			Inverted: false,
		})
	}

	switch yt.Kind {
	case yang.Yunion:
		for _, ytt := range yt.Type {
			schem, err := sc.toSchemaType(e, ytt)
			if err != nil {
				return nil, err
			}
			slt.UnionTypes = append(slt.UnionTypes, schem)
		}
	case yang.Yidentityref:
		if yt.IdentityBase == nil {
			panic("expected identityref type to have IdentityBase")
		}

		slt.IdentityPrefixesMap = make(map[string]string, len(yt.IdentityBase.Values))
		slt.ModulePrefixMap = make(map[string]string, len(yt.IdentityBase.Values))
		for _, identity := range yt.IdentityBase.Values {
			identityRoot := yang.RootNode(identity)
			slt.IdentityPrefixesMap[identity.Name] = identityRoot.GetPrefix()
			slt.ModulePrefixMap[identity.Name] = identityRoot.NName()
		}
	case yang.Yleafref:
		var err error
		normalizedPath := normalizePath(yt.Path, e)
		leafSchemaEntry, err := sc.GetEntry(normalizedPath)
		if err != nil {
			return nil, err
		}
		lsePath := leafSchemaEntry.Path()
		_ = lsePath
		slt.LeafrefTargetType, err = sc.toSchemaType(leafSchemaEntry, leafSchemaEntry.Type)
		if err != nil {
			return nil, err
		}
	}

	return slt, nil
}

func yRangeToSchemaMinMaxType(r *yang.YRange) *sdcpb.SchemaMinMaxType {
	return &sdcpb.SchemaMinMaxType{Min: &sdcpb.Number{Value: r.Min.Value, Negative: r.Min.Negative}, Max: &sdcpb.Number{Value: r.Max.Value, Negative: r.Max.Negative}}
}

func getMustStatement(e *yang.Entry) []*sdcpb.MustStatement {
	mustStatements, ok := e.Extra["must"]
	if !ok {
		return nil
	}
	rs := make([]*sdcpb.MustStatement, 0, len(mustStatements))
	for _, m := range mustStatements {
		if m, ok := m.(*yang.Must); ok {
			// newlines might appear in the yang file, replace them with space
			stmt := strings.ReplaceAll(m.Name, "\n", " ")
			ms := &sdcpb.MustStatement{
				Statement: stmt,
			}
			if m.ErrorMessage != nil {
				ms.Error = m.ErrorMessage.Name
			}
			rs = append(rs, ms)
		}
	}
	return rs
}

func getIfFeature(e *yang.Entry) []string {
	ifFeatures, ok := e.Extra["if-feature"]
	if !ok {
		return nil
	}
	rs := make([]string, 0, len(ifFeatures))
	for _, m := range ifFeatures {
		if m, ok := m.(*yang.Value); ok {
			rs = append(rs, m.Name)
		}
	}
	return rs
}
