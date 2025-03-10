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
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/utils"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
)

func (sc *Schema) SchemaElemFromYEntry(e *yang.Entry, withDesc bool) (*sdcpb.SchemaElem, error) {
	switch {
	case e.IsLeaf():
		leaf, err := sc.leafFromYEntry(e, withDesc)
		if err != nil {
			return nil, err
		}
		return &sdcpb.SchemaElem{
			Schema: &sdcpb.SchemaElem_Field{
				Field: leaf,
			},
		}, nil
	case e.IsLeafList():
		leaflist, err := sc.leafListFromYEntry(e, withDesc)
		if err != nil {
			return nil, err
		}
		return &sdcpb.SchemaElem{
			Schema: &sdcpb.SchemaElem_Leaflist{
				Leaflist: leaflist,
			},
		}, nil
	default:
		container, err := sc.containerFromYEntry(e, withDesc)
		if err != nil {
			return nil, err
		}
		return &sdcpb.SchemaElem{
			Schema: &sdcpb.SchemaElem_Container{
				Container: container,
			},
		}, nil
	}
}

func (sc *Schema) GetEntry(pe []string) (*yang.Entry, error) {
	if len(pe) == 0 {
		return sc.root, nil
	}
	// TODO: code needs to be refactored.
	// Following piece of code takes both normalized paths or module-prepended paths as input.
	// for example:
	//   []string{"srl_nokia-if:interface", "srl_nokia-if:name"}
	//   []string{"interface", "name"}

	sc.m.RLock()
	defer sc.m.RUnlock()

	// Assume we are always dealing with an absolute path here?
	mods, err := sc.FindPossibleModulesForPathElement(sc.root, pe[0])
	if err != nil {
		return nil, err
	}
	for i, mod := range mods {
		entry, err := getEntry(mod, pe)
		if err == nil {
			return entry, nil
		}
		remainingMods := make([]string, 0, len(mods)-(i+1))
		for _, rMod := range mods[i+1:] {
			remainingMods = append(remainingMods, rMod.Name)
		}
		log.Debugf("looking up path %s in module %s caused: %v. continuing to search in %v", strings.Join(pe, "/"), mod.Name, err, remainingMods)
	}
	// if we are here we have not found a path, maybe we have a module name
	// if we have one module and one path element, likely a module return this
	if len(mods) == 1 && len(pe) == 1 && mods[0].Name == pe[0] {
		return mods[0], nil
	}
	return nil, fmt.Errorf("schema entry %q not found", strings.Join(pe, "/"))
}

func (sc *Schema) FindPossibleModulesForPathElement(e *yang.Entry, pathElement string) ([]*yang.Entry, error) {
	if e == nil {
		return nil, errors.New("nil yang entry")
	}

	prefix, path, foundPrefix := strings.Cut(pathElement, ":")
	if !foundPrefix {
		path = prefix
		prefix = ""
	}
	// try to shortcut by returning the module directly if pathElement matches the name/prefix
	if mod, ok := e.Dir[path]; ok && (!foundPrefix || mod.Prefix.Name == prefix) {
		return []*yang.Entry{mod}, nil
	}

	switch {
	case e.Node == nil && e.Name != RootName:
		return nil, fmt.Errorf("entry has no Node but is not root, instead is %s", e.Name)
	case e.Node == nil:
		entries := make([]*yang.Entry, 0)
		for _, entry := range sc.root.Dir {
			if ee, ok := entry.Dir[path]; ok && (!foundPrefix || ee.Prefix.Name == prefix) {
				entries = append(entries, entry)
			}
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("unable to find entry for prefix %q, path %q in root", prefix, path)
		}
		sort.Slice(entries, func(i, j int) bool {
			return utils.SortModulesAB(entries[i].Name, entries[j].Name, config.DeprioritizedModules)
		})
		return entries, nil

	default:
		mod := yang.RootNode(e.Node)
		if prefix == mod.GetPrefix() {
			foundEntry, ok := sc.root.Dir[mod.Name]
			if !ok {
				return nil, fmt.Errorf("module %q not found in loaded modules", mod.Name)
			}
			if _, ok := foundEntry.Dir[path]; ok {
				return []*yang.Entry{foundEntry}, nil
			}
		}

		for _, i := range mod.Import {
			if prefix == i.Prefix.Name {
				foundEntry, ok := sc.root.Dir[i.Name]
				if !ok {
					return nil, fmt.Errorf("module %q not found in loaded modules", i.Name)
				}
				if _, ok := foundEntry.Dir[path]; ok {
					return []*yang.Entry{foundEntry}, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("error getting Element for pathElement %q", pathElement)
}

func getEntry(e *yang.Entry, pe []string) (*yang.Entry, error) {
	log.Tracef("getEntry %s Dir=%v, Choice=%v, Case=%v, %v",
		e.Name,
		e.IsDir(),
		e.IsChoice(),
		e.IsCase(),
		pe)
	switch len(pe) {
	case 0:
		switch {
		case e.IsCase(), e.IsChoice():
			if ee := e.Dir[e.Name]; ee != nil {
				return ee, nil
			}
			// case e.IsContainer():
			// 	if ee := e.Dir[e.Name]; ee != nil {
			// 		if ee.IsCase() || ee.IsChoice() {
			// 			return ee, nil
			// 		}
			// 	}
		}
		return e, nil
	default:
		if e.Dir == nil {
			return nil, errors.New("not found")
		}
		for _, ee := range getChildren(e) {
			// fmt.Printf("entry %s, child %s | %s\n", e.Name, ee.Name, pe)

			// prefix will be in [0] if exists, so path will always be in last index
			// compare name without prefix
			pathElements := strings.SplitN(pe[0], ":", 2)
			if ee.Name != pathElements[len(pathElements)-1] {
				continue
			}
			return getEntry(ee, pe[1:])
		}
		// fmt.Println("entry name", e.Name, pe)
		return nil, fmt.Errorf("%q not a child entry of %v", pe[0], e.Name)
	}
}

func (sc *Schema) BuildPath(pe []string, p *sdcpb.Path) error {
	if len(pe) == 0 {
		return nil
	}
	sc.m.RLock()
	defer sc.m.RUnlock()
	if p.GetElem() == nil {
		p.Elem = make([]*sdcpb.PathElem, 0, 1)
	}
	first := pe[0]
	index := strings.Index(pe[0], ":")
	if index > 0 {
		first = pe[0][:index]
		pe[0] = pe[0][index+1:]
	}
	// try module
	if e, ok := sc.root.Dir[first]; ok {
		if e == nil {
			return fmt.Errorf("module %q not found", first)
		}
		if ee, ok := e.Dir[pe[0]]; ok {
			err := sc.buildPath(pe, p, ee)
			if err != nil {
				return err
			}
			// add ns/prefix to the first elem
			p.GetElem()[0].Name = first + ":" + p.GetElem()[0].GetName()
			return nil
		}
		return fmt.Errorf("elem %q not found in module %q", pe[0], first)
	}
	// try children
	for _, e := range sc.root.Dir {
		if ee, ok := e.Dir[pe[0]]; ok {
			return sc.buildPath(pe, p, ee)
		}
	}

	return fmt.Errorf("path %v does not exist in schema %s", pe, sc.config.GetSchema().String())
}

func (sc *Schema) buildPath(pe []string, p *sdcpb.Path, e *yang.Entry) error {
	log.Tracef("buildPath START")
	log.Tracef("buildPath: remainingPathElems=%v, path=%v", pe, p)
	log.Tracef("received PE=%v", pe)
	log.Tracef("current path=%v", p)
	log.Tracef("YANG entry=%v isChoice=%v, isCase=%v", e.Name, e.IsChoice(), e.IsCase())
	log.Tracef("YANG children: %v", e.Dir)
	log.Tracef("buildPath END")

	lpe := len(pe)
	cpe := &sdcpb.PathElem{
		Name: e.Name,
		Key:  make(map[string]string),
	}
	if lpe == 0 {
		p.Elem = append(p.Elem, cpe)
		return nil
	}

	switch {
	case e.IsList():
		if cpe.GetKey() == nil {
			cpe.Key = make(map[string]string)
		}
		p.Elem = append(p.Elem, cpe)
		keys := strings.Fields(e.Key)
		sort.Strings(keys)
		count := 1
		for i, k := range keys {
			if i+1 >= lpe {
				break
			}
			count++
			cpe.Key[k] = pe[i+1]
		}
		if lpe == count {
			return nil
		}
		nxt := pe[count]
		if ee, ok := e.Dir[nxt]; ok {
			return sc.buildPath(pe[count:], p, ee)
		}
		// find choices/cases
		ee, err := sc.findChoiceCase(e, pe[count-1:])
		if err != nil {
			return fmt.Errorf("list %s - %v", e.Name, err)
		}
		return sc.buildPath(pe[count:], p, ee)
	case e.IsChoice():
		p.Elem = append(p.Elem, cpe)
		for _, entry := range e.Dir {
			if entry.IsCase() {
				if ee, ok := entry.Dir[pe[0]]; ok {
					return sc.buildPath(pe[1:], p, ee)
				}
			} else {
				if ee, ok := e.Dir[pe[0]]; ok {
					return sc.buildPath(pe[1:], p, ee)
				}
			}
		}
		return fmt.Errorf("choice %s - unknown element %s", e.Name, pe[0])
	case e.IsCase():
		// RFC7950 7.9.2: A case node does not exist in the data tree.
		// p.Elem = append(p.Elem, cpe)
		if ee, ok := e.Dir[pe[0]]; ok {
			return sc.buildPath(pe[1:], p, ee)
		}
		if ee, ok := e.Dir[e.Name]; ok {
			return sc.buildPath(pe, p, ee)
		}
		return fmt.Errorf("case %s - unknown element %s", e.Name, pe[0])
	case e.IsContainer():
		// implicit case: child with same name which is a choice
		if ee, ok := e.Dir[pe[0]]; ee != nil && ok {
			if ee.IsChoice() {
				return sc.buildPath(pe[1:], p, ee)
			}
		}

		p.Elem = append(p.Elem, cpe)
		if ee, ok := e.Dir[pe[0]]; ok {
			return sc.buildPath(pe, p, ee)
		}
		if lpe == 1 {
			return nil
		}
		if ee, ok := e.Dir[pe[1]]; ok {
			return sc.buildPath(pe[1:], p, ee)
		}
		// find choice/case
		ee, err := sc.findChoiceCase(e, pe)
		if err != nil {
			return fmt.Errorf("container %s - %v", e.Name, err)
		}
		return sc.buildPath(pe[1:], p, ee)
	case e.IsLeaf():
		if lpe != 1 {
			return fmt.Errorf("leaf %s - unknown element %v", e.Name, pe[0])
		}
		p.Elem = append(p.Elem, cpe)
	case e.IsLeafList():
		p.Elem = append(p.Elem, cpe)
		switch lpe {
		case 1:
		case 2:
			cpe.Key[cpe.Name] = pe[1]
		default:
			return fmt.Errorf("leafList %s - unknown element %q", e.Name, pe[2])
		}
	}
	return nil
}

func getChildren(e *yang.Entry) []*yang.Entry {
	switch {
	case e.IsChoice(), e.IsCase(), e.IsContainer(), e.IsList():
		rs := make([]*yang.Entry, 0, len(e.Dir))
		for _, ee := range e.Dir {
			if ee.IsChoice() || ee.IsCase() {
				rs = append(rs, getChildren(ee)...)
				continue
			}
			rs = append(rs, ee)
		}
		//sort.Slice(rs, sortFn(rs))
		return rs
		// case e.IsCase():
		// 	rs := make([]*yang.Entry, 0, len(e.Dir))
		// 	// if len(e.Dir) == 0 {
		// 	// 	rs = append(rs, e)
		// 	// 	return rs
		// 	// }

		// 	for _, ee := range e.Dir {
		// 		if ee.IsChoice() || ee.IsCase() {
		// 			rs = append(rs, getChildren(ee)...)
		// 			continue
		// 		}
		// 		rs = append(rs, ee)
		// 	}
		// 	//sort.Slice(rs, sortFn(rs))
		// 	return rs
		// case e.IsContainer():
		// 	rs := make([]*yang.Entry, 0, len(e.Dir))
		// 	for _, ee := range e.Dir {
		// 		if ee.IsChoice() || ee.IsCase() {
		// 			rs = append(rs, getChildren(ee)...)
		// 			continue
		// 		}
		// 		rs = append(rs, ee)
		// 	}
		// 	//sort.Slice(rs, sortFn(rs))
		// 	return rs
		// case e.IsList():
		// 	rs := make([]*yang.Entry, 0, len(e.Dir))
		// 	for _, ee := range e.Dir {
		// 		if ee.IsChoice() || ee.IsCase() {
		// 			rs = append(rs, getChildren(ee)...)
		// 			continue
		// 		}
		// 		rs = append(rs, ee)
		// 	}
		// 	//sort.Slice(rs, sortFn(rs))
		// 	return rs
	default:
		return nil
	}
}

func getParent(e *yang.Entry) *yang.Entry {
	if e == nil {
		return nil
	}
	if e.Parent != nil && e.Parent.Name == RootName {
		return nil
	}
	// if !e.IsChoice() && !e.IsCase() {
	// 	return e.Parent
	// }
	// if e.Parent.IsCase() || e.Parent.IsChoice() {
	// 	fmt.Println("getParent Parent", e.Parent.Name, e.Parent.IsChoice(), e.Parent.IsCase())
	// 	return getParent(e.Parent)
	// }
	return e.Parent
}

func sortFn(rs []*yang.Entry) func(i, j int) bool {
	return func(i, j int) bool {
		switch {
		case rs[i].IsLeaf():
			switch {
			case rs[j].IsLeaf():
				return rs[i].Name < rs[j].Name
			default:
				return true
			}
		case rs[i].IsChoice():
			switch {
			case rs[j].IsLeaf():
				return false
			case rs[j].IsChoice():
				return rs[i].Name < rs[j].Name
			default:
				return true
			}
		case rs[i].IsCase():
			switch {
			case rs[j].IsLeaf():
				return false
			case rs[j].IsChoice():
				return false
			// case rs[j].IsLeafList():
			// 	return false
			case rs[j].IsCase():
				return rs[i].Name < rs[j].Name
			default:
				return true
			}
		case rs[i].IsContainer():
			switch {
			case rs[j].IsContainer():
				return rs[i].Name < rs[j].Name
			default:
				return false
			}
		default:
			return false
		}
	}
}

// ch

func (sc *Schema) GetEntryCh(pe []string, ch chan *yang.Entry) error {
	defer close(ch)
	if len(pe) == 0 {
		ch <- sc.root
		return nil
	}
	first := pe[0]
	offset := 1
	index := strings.Index(pe[0], ":")
	if index > 0 {
		first = pe[0][:index]
		pe[0] = pe[0][index+1:]
		offset = 0
	}

	sc.m.RLock()
	defer sc.m.RUnlock()
	if e, ok := sc.root.Dir[first]; ok {
		if e == nil {
			return fmt.Errorf("module %q not found", first)
		}
		return getEntryCh(e, pe[offset:], ch)
	}
	// skip first level modules and try their children
	for _, child := range sc.root.Dir {
		if cc, ok := child.Dir[first]; ok {
			ch <- cc
			return getEntryCh(cc, pe[offset:], ch)
		}
	}
	return fmt.Errorf("entry %q not found", pe[0])
}

func getEntryCh(e *yang.Entry, pe []string, ch chan *yang.Entry) error {
	log.Tracef("getEntryCh: %v ", pe)
	log.Tracef("getEntryCh %s Dir=%v, Choice=%v, Case=%v, %v",
		e.Name,
		e.IsDir(),
		e.IsChoice(),
		e.IsCase(),
		pe)
	switch len(pe) {
	case 0:
		switch {
		case e.IsCase(), e.IsChoice():
			if ee := e.Dir[e.Name]; ee != nil {
				ch <- ee
				return nil
			}
			// case e.IsContainer():
			// 	if ee := e.Dir[e.Name]; ee != nil {
			// 		if ee.IsCase() || ee.IsChoice() {
			// 			//ch <- ee
			// 			return nil
			// 		}
			// 	}
		}
		return nil
	default:
		if e.Dir == nil {
			return errors.New("not found")
		}
		for _, ee := range getChildren(e) {
			// fmt.Printf("entry %s, child %s | %s\n", e.Name, ee.Name, pe)
			if ee.Name != pe[0] {
				continue
			}
			log.Debugf("%v , %q | Dir=%v,Cont=%v Choice=%v, Case=%v\n", pe, ee.Name,
				e.IsDir(),
				e.IsContainer(),
				e.IsChoice(),
				e.IsCase())

			ch <- ee
			return getEntryCh(ee, pe[1:], ch)
		}
		// fmt.Println("entry name", e.Name, pe)
		return fmt.Errorf("%q not found", pe[0])
	}
}

// findChoiceCase finds an entry nested in a choice and potentially a case statement
func (sc *Schema) findChoiceCase(e *yang.Entry, pe []string) (*yang.Entry, error) {
	if len(pe) == 0 {
		return e, nil
	}
	for _, ee := range e.Dir {
		if !ee.IsChoice() {
			continue
		}
		if eee, ok := ee.Dir[pe[1]]; ok && !eee.IsCase() {
			return eee, nil
		}
		// assume there was a case obj,
		// search one step deeper
		for _, eee := range ee.Dir {
			if !eee.IsCase() {
				continue
			}
			if eeee, ok := eee.Dir[pe[1]]; ok {
				return eeee, nil
			}
		}
	}
	return nil, fmt.Errorf("unknown element %s", pe[1])
}
