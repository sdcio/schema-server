package schema

import (
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/openconfig/goyang/pkg/yang"
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
		ChoiceInfo:     getChoiceInfo(e),
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
	}
	return ll
}
