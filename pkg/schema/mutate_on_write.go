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
)

const deviateExtsAnnotationKey = "DEVIATE_EXTS"

type qualifiedIdentifier struct {
	module string
	name   string
}

var mutateOnWriteTypedefs = map[qualifiedIdentifier]struct{}{
	{module: "openconfig-aaa-types", name: "crypt-password-type"}:       {},
	{module: "iana-crypt-hash", name: "crypt-hash"}:                     {},
	{module: "srl_nokia-common", name: "crypt-password-type"}:           {},
	{module: "tailf-common", name: "md5-digest-string"}:                 {},
	{module: "tailf-common", name: "sha-256-digest-string"}:             {},
	{module: "tailf-common", name: "sha-512-digest-string"}:             {},
	{module: "tailf-common", name: "des3-cbc-encrypted-string"}:         {},
	{module: "tailf-common", name: "aes-cfb-128-encrypted-string"}:      {},
	{module: "tailf-common", name: "aes-256-cfb-128-encrypted-string"}:  {},
	{module: "Cisco-IOS-XR-types", name: "Proprietary-password"}:        {},
	{module: "Cisco-IOS-XR-types", name: "Md5-password"}:                {},
	{module: "Cisco-IOS-XR-types", name: "Type6-password"}:              {},
	{module: "Cisco-IOS-XR-types", name: "Type8-password"}:              {},
	{module: "Cisco-IOS-XR-types", name: "Type9-password"}:              {},
	{module: "Cisco-IOS-XR-types", name: "Type10-password"}:             {},
}

// srl_nokia-common:routing-password carries SecretKey internally in SR Linux
// YANG for UI censoring even though the target returns cleartext on readback.
var cleartextExcludedTypedefs = map[qualifiedIdentifier]struct{}{
	{module: "openconfig-types", name: "routing-password"}: {},
	{module: "srl_nokia-common", name: "routing-password"}: {},
}

// scopedExtension pairs a YANG extension statement with the AST node whose
// enclosing module imports govern prefix resolution for stmt.Keyword.
type scopedExtension struct {
	context yang.Node
	stmt    *yang.Statement
}

// buildDeviateAnnotations attaches extension statements added via YANG
// deviate add overlays onto target entries alongside the overlay's AST scope.
func (sc *Schema) buildDeviateAnnotations() {
	if sc.modules == nil {
		return
	}
	visited := make(map[*yang.Module]bool)
	for _, modMap := range []map[string]*yang.Module{sc.modules.Modules, sc.modules.SubModules} {
		for _, mod := range modMap {
			if visited[mod] {
				continue
			}
			visited[mod] = true
			modEntry := yang.ToEntry(mod)
			for _, devStmt := range mod.Deviation {
				target := modEntry.Find(devStmt.Name)
				if target == nil {
					continue
				}
				for _, dev := range devStmt.Deviate {
					if dev.Name != "add" || len(dev.Exts()) == 0 {
						continue
					}
					if target.Annotation == nil {
						target.Annotation = make(map[string]interface{})
					}
					added, _ := target.Annotation[deviateExtsAnnotationKey].([]scopedExtension)
					for _, ext := range dev.Exts() {
						added = append(added, scopedExtension{context: dev, stmt: ext})
					}
					target.Annotation[deviateExtsAnnotationKey] = added
				}
			}
		}
	}
}

// isMutateOnWrite reports whether leaf or leaf-list entry e carries a
// mutate-on-write annotation or type — directly, via a deviate add overlay, on
// an inline type, across a typedef or leafref chain, or in any union member —
// while keeping UI-only echo suppression and cleartext password types false.
func (sc *Schema) isMutateOnWrite(e *yang.Entry) bool {
	c := mutateOnWriteClassifier{
		sc:              sc,
		visitedEntries:  make(map[*yang.Entry]bool),
		visitedTypedefs: make(map[*yang.Typedef]bool),
	}
	return c.classifyEntry(e)
}

// mutateOnWriteClassifier walks YANG entries, AST types, typedef chains,
// unions, and leafrefs while guarding against cyclic references.
type mutateOnWriteClassifier struct {
	sc              *Schema
	ownerEntry      *yang.Entry
	visitedEntries  map[*yang.Entry]bool
	visitedTypedefs map[*yang.Typedef]bool
}

func (c *mutateOnWriteClassifier) classifyEntry(e *yang.Entry) bool {
	if e == nil || c.visitedEntries[e] {
		return false
	}
	c.visitedEntries[e] = true
	defer delete(c.visitedEntries, e)

	prevOwner := c.ownerEntry
	c.ownerEntry = e
	defer func() { c.ownerEntry = prevOwner }()

	instModNode := entryRootNode(e)
	for _, stmt := range e.Exts {
		if isMutateOnWriteExtensionWithFallback(e.Node, instModNode, stmt) {
			return true
		}
	}
	if e.Annotation != nil {
		if added, ok := e.Annotation[deviateExtsAnnotationKey].([]scopedExtension); ok {
			for _, ext := range added {
				if isMutateOnWriteExtension(ext.context, ext.stmt) {
					return true
				}
			}
		}
	}
	if astType := entryASTType(e); astType != nil {
		return c.classifyASTType(astType)
	}
	return c.classifyYangType(e.Type, true)
}

func (c *mutateOnWriteClassifier) classifyASTType(t *yang.Type) bool {
	if t == nil {
		return false
	}
	if hasMutateOnWriteNodeExts(t, t.Exts()) {
		return true
	}
	for _, memberType := range t.Type {
		if c.classifyASTType(memberType) {
			return true
		}
	}
	return c.classifyYangType(t.YangType, false)
}

func (c *mutateOnWriteClassifier) classifyYangType(yt *yang.YangType, includeUnionMembers bool) bool {
	if yt == nil {
		return false
	}
	if includeUnionMembers && yt.Kind == yang.Yunion {
		for _, member := range yt.Type {
			if c.classifyYangType(member, true) {
				return true
			}
		}
	}
	if yt.Kind == yang.Yleafref && yt.Path != "" {
		if c.classifyLeafref(yt.Path) {
			return true
		}
	}
	if yt.Base != nil {
		if td, ok := yt.Base.Parent.(*yang.Typedef); ok && td != nil && td.Parent != nil {
			return c.classifyTypedef(td)
		}
	}
	return false
}

func (c *mutateOnWriteClassifier) classifyLeafref(rawPath string) bool {
	if c.sc == nil || c.ownerEntry == nil || rawPath == "" {
		return false
	}
	normalized := normalizePath(rawPath, c.ownerEntry)
	targetEntry, err := c.sc.GetEntry(normalized)
	if err != nil || targetEntry == nil {
		return false
	}
	return c.classifyEntry(targetEntry)
}

func (c *mutateOnWriteClassifier) classifyTypedef(td *yang.Typedef) bool {
	if c.visitedTypedefs[td] {
		return false
	}
	c.visitedTypedefs[td] = true
	defer delete(c.visitedTypedefs, td)

	if mod := owningModule(yang.RootNode(td)); mod != nil {
		id := qualifiedIdentifier{module: mod.Name, name: td.Name}
		if _, excluded := cleartextExcludedTypedefs[id]; excluded {
			return false
		}
		if _, matched := mutateOnWriteTypedefs[id]; matched {
			return true
		}
	}
	if hasMutateOnWriteNodeExts(td, td.Exts()) {
		return true
	}
	return c.classifyASTType(td.Type)
}

func entryASTType(e *yang.Entry) *yang.Type {
	switch n := e.Node.(type) {
	case *yang.Leaf:
		return n.Type
	case *yang.LeafList:
		return n.Type
	}
	return nil
}

func entryRootNode(e *yang.Entry) yang.Node {
	curr := e
	for curr != nil && curr.Parent != nil && curr.Parent.Name != RootName {
		curr = curr.Parent
	}
	if curr == nil {
		return nil
	}
	return curr.Node
}

func hasMutateOnWriteNodeExts(ctx yang.Node, stmts []*yang.Statement) bool {
	for _, stmt := range stmts {
		if isMutateOnWriteExtension(ctx, stmt) {
			return true
		}
	}
	return false
}

func isMutateOnWriteExtension(ctx yang.Node, stmt *yang.Statement) bool {
	return isMutateOnWriteExtensionWithFallback(ctx, nil, stmt)
}

func isMutateOnWriteExtensionWithFallback(primaryCtx, fallbackCtx yang.Node, stmt *yang.Statement) bool {
	if stmt == nil {
		return false
	}
	mod, ident := resolveExtensionModule(primaryCtx, stmt)
	if mod == nil && fallbackCtx != nil && fallbackCtx != primaryCtx {
		mod, ident = resolveExtensionModule(fallbackCtx, stmt)
	}
	if mod == nil || ident == "" {
		return false
	}
	if ident == "mutate-on-write" && mod.Name == "sdcio-ext" && mod.Namespace != nil && mod.Namespace.Name == "urn:sdcio-ext" {
		return true
	}
	switch mod.Name {
	case "openconfig-extensions":
		return ident == "openconfig-hashed-value"
	case "srl_nokia-extensions":
		switch ident {
		case "hash-algorithm":
			return true
		case "generated-class-name":
			return stmt.Argument == "PasswordHash" || stmt.Argument == "SecretKey"
		}
	}
	return false
}

func resolveExtensionModule(ctx yang.Node, stmt *yang.Statement) (*yang.Module, string) {
	if ctx == nil || stmt == nil {
		return nil, ""
	}
	prefix, ident, ok := strings.Cut(stmt.Keyword, ":")
	if !ok {
		ident = prefix
		prefix = ""
	}
	mod := owningModule(yang.FindModuleByPrefix(ctx, prefix))
	return mod, ident
}

func owningModule(mod *yang.Module) *yang.Module {
	if mod == nil {
		return nil
	}
	if mod.Kind() == "submodule" && mod.BelongsTo != nil && mod.Modules != nil {
		if parent := mod.Modules.Modules[mod.BelongsTo.Name]; parent != nil {
			return parent
		}
	}
	return mod
}
