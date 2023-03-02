package types

import "github.com/beevik/etree"

type NetconfResponse struct {
	Doc *etree.Document
}

func NewNetconfResponse(doc *etree.Document) *NetconfResponse {
	return &NetconfResponse{
		Doc: doc,
	}
}
