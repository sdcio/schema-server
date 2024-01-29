package schema

import (
	"sort"
	"strings"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/openconfig/goyang/pkg/yang"
)

func containerFromYEntry(e *yang.Entry, withDesc bool) *sdcpb.ContainerSchema {
	c := &sdcpb.ContainerSchema{
		Name:           e.Name,
		Namespace:      e.Namespace().Name,
		Keys:           []*sdcpb.LeafSchema{},
		Fields:         []*sdcpb.LeafSchema{},
		Leaflists:      []*sdcpb.LeafListSchema{},
		Children:       []string{},
		MustStatements: getMustStatement(e),
		IsState:        isState(e),
		IsPresence:     isPresence(e),
		ChoiceInfo:     getChoiceInfo(e),
		IfFeature:      getIfFeature(e),
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
	}

	keys := map[string]struct{}{}
	for _, key := range strings.Fields(e.Key) {
		keys[key] = struct{}{}
	}
	for _, child := range getChildren(e) {
		switch {
		case child.IsDir():
			c.Children = append(c.Children, child.Name)
		case child.IsLeaf(), child.IsLeafList():
			o := SchemaElemFromYEntry(child, withDesc)
			switch o := o.Schema.(type) {
			case *sdcpb.SchemaElem_Field:
				if _, ok := keys[child.Name]; ok {
					c.Keys = append(c.Keys, o.Field)
					continue
				}
				c.Fields = append(c.Fields, o.Field)
			case *sdcpb.SchemaElem_Leaflist:
				c.Leaflists = append(c.Leaflists, o.Leaflist)
			}
		}
	}
	sort.Strings(c.Children)
	sort.Slice(c.Keys, func(i, j int) bool {
		return c.Keys[i].GetName() < c.Keys[j].GetName()
	})
	return c
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
	if e == nil || e.Parent == nil || e.Parent.Parent == nil {
		return nil
	}
	var choice, cas *yang.Entry
	if e.Parent.IsCase() {
		cas = e.Parent
	}
	if e.Parent.Parent.IsChoice() {
		choice = e.Parent.Parent
	}
	if choice == nil && cas == nil {
		return nil
	}
	ci := &sdcpb.ChoiceInfo{
		Choice:  choice.Name,
		Case:    cas.Name,
		AltCase: []string{},
	}
	for _, ee := range choice.Dir {
		if !ee.IsCase() {
			continue
		}
		for _, eee := range ee.Dir {
			ci.AltCase = append(ci.AltCase, strings.Join([]string{ee.Name, eee.Name}, "/"))
		}
	}
	return ci
}
