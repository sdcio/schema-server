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
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"

	"github.com/sdcio/schema-server/pkg/utils"
)

func (sc *Schema) ExpandPath(p *sdcpb.Path, dt sdcpb.DataType) ([]*sdcpb.Path, error) {
	ps := make([]*sdcpb.Path, 0)
	cp := utils.ToStrings(p, false, true)
	e, err := sc.GetEntry(cp)
	if err != nil {
		return nil, err
	}
	populatePathKeys(e, p)
	switch {
	case e.IsLeaf():
		return []*sdcpb.Path{p}, nil
	}
	keys := map[string]struct{}{}
	for _, k := range strings.Fields(e.Key) {
		keys[k] = struct{}{}
	}
	for _, c := range e.Dir {
		// skip keys
		if _, ok := keys[c.Name]; ok {
			continue
		}
		for _, pe := range sc.getPathElems(c, dt) {
			np := &sdcpb.Path{
				Elem: make([]*sdcpb.PathElem, 0, len(p.GetElem())+len(pe)),
			}
			np.Elem = append(np.Elem, p.GetElem()...)
			np.Elem = append(np.Elem, pe...)
			ps = append(ps, np)
		}
	}
	return ps, nil
}

func (sc *Schema) getPathElems(e *yang.Entry, dt sdcpb.DataType) [][]*sdcpb.PathElem {
	rs := make([][]*sdcpb.PathElem, 0)
	switch {
	case e.IsCase():
		log.Debugf("got case: %s", e.Name)
		for _, c := range e.Dir {
			rs = append(rs, sc.getPathElems(c, dt)...)
		}
	case e.IsChoice():
		log.Debugf("got choice: %s", e.Name)
		for _, c := range e.Dir {
			rs = append(rs, sc.getPathElems(c, dt)...)
		}
	case e.IsLeaf():
		log.Debugf("got leaf: %s", e.Name)
		switch dt {
		case sdcpb.DataType_ALL:
		case sdcpb.DataType_CONFIG:
			if isState(e) {
				return nil
			}
		case sdcpb.DataType_STATE:
			if !isState(e) {
				return nil
			}
		}
		return [][]*sdcpb.PathElem{{&sdcpb.PathElem{Name: e.Name}}}
	case e.IsLeafList():
		log.Debugf("got leafList: %s", e.Name)
		switch dt {
		case sdcpb.DataType_ALL:
		case sdcpb.DataType_CONFIG:
			if isState(e) {
				return nil
			}
		case sdcpb.DataType_STATE:
			if !isState(e) {
				return nil
			}
		}
		return [][]*sdcpb.PathElem{{&sdcpb.PathElem{Name: e.Name}}}
	case e.IsList():
		log.Debugf("got list: %s", e.Name)
		listPE := &sdcpb.PathElem{Name: e.Name, Key: make(map[string]string)}
		keys := strings.Fields(e.Key)
		kmap := make(map[string]struct{})
		for _, k := range keys {
			listPE.Key[k] = "*"
			kmap[k] = struct{}{}
		}

		for _, c := range e.Dir {
			if _, ok := kmap[c.Name]; ok {
				continue
			}
			log.Debugf("list parent adding child: %s", c.Name)

			childrenPE := sc.getPathElems(c, dt)
			for _, cpe := range childrenPE {
				branch := make([]*sdcpb.PathElem, 0, len(cpe)+1)
				branch = append(branch, listPE)
				branch = append(branch, cpe...)
				rs = append(rs, branch)
			}
		}
	case e.IsContainer():
		log.Debugf("got container: %s", e.Name)
		containerPE := &sdcpb.PathElem{Name: e.Name, Key: make(map[string]string)}
		for _, c := range e.Dir {
			log.Debugf("container parent adding child: %s", c.Name)
			childrenPE := sc.getPathElems(c, dt)

			for _, cpe := range childrenPE {
				branch := make([]*sdcpb.PathElem, 0, len(cpe)+1)
				branch = append(branch, containerPE)
				branch = append(branch, cpe...)
				rs = append(rs, branch)
			}
		}
	}
	return rs
}

func populatePathKeys(e *yang.Entry, p *sdcpb.Path) {
	ce := e
	for i := len(p.GetElem()) - 1; i >= 0; i-- {
		if ce.Parent != nil && ce.Parent.Name == RootName {
			return
		}
		populatePathElemKeys(ce, p.GetElem()[i])
		ce = ce.Parent
	}
}

func populatePathElemKeys(e *yang.Entry, pe *sdcpb.PathElem) {
	switch {
	case e.IsList():
		keys := strings.Fields(e.Key)
		sort.Strings(keys)
		for _, k := range keys {
			if pe.GetKey() == nil {
				pe.Key = make(map[string]string)
			}
			if _, ok := pe.GetKey()[k]; !ok {
				pe.Key[k] = "*"
			}
		}
	}
}
