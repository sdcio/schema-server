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
	"sync"
	"time"

	"github.com/openconfig/goyang/pkg/yang"
	"github.com/sdcio/schema-server/pkg/config"
	log "github.com/sirupsen/logrus"
)

const (
	RootName = "__root__"
)

type Schema struct {
	config *config.SchemaConfig

	m       *sync.RWMutex
	root    *yang.Entry
	modules *yang.Modules
	status  string
}

func NewSchema(sCfg *config.SchemaConfig) (*Schema, error) {
	sc := &Schema{
		config:  sCfg,
		m:       new(sync.RWMutex),
		root:    &yang.Entry{},
		modules: yang.NewModules(),
	}
	now := time.Now()
	var err error
	sCfg.Files, err = findYangFiles(sCfg.Files)
	if err != nil {
		sc.status = "failed"
		return sc, err
	}
	err = sc.readYANGFiles()
	if err != nil {
		sc.status = "failed"
		return sc, err
	}
	sc.root = &yang.Entry{
		Name: RootName,
		Kind: yang.DirectoryEntry,
		Dir:  make(map[string]*yang.Entry, len(sc.modules.Modules)),
		Annotation: map[string]interface{}{
			"schemapath": "/",
			"root":       true,
		},
	}

	for _, m := range sc.modules.Modules {
		e := yang.ToEntry(m)
		sc.root.Dir[e.Name] = e
	}
	log.Infof("schema %s building references", sc.UniqueName(""))
	err = sc.buildReferencesAnnotation()
	if err != nil {
		return nil, err
	}
	sc.status = "ok"
	log.Infof("schema %s parsed in %s", sc.UniqueName(""), time.Since(now))
	sc.modules = nil
	return sc, nil
}

func (s *Schema) Reload() (*Schema, error) {
	s.status = "reloading"
	return NewSchema(s.config)
}

func (s *Schema) UniqueName(sep string) string {
	if s == nil {
		return ""
	}
	if sep == "" {
		sep = "@"
	}
	return strings.Join([]string{s.config.Name, s.config.Vendor, s.config.Version}, sep)
}

func (s *Schema) Name() string {
	if s == nil {
		return ""
	}
	return s.config.Name
}

func (s *Schema) Vendor() string {
	if s == nil {
		return ""
	}
	return s.config.Vendor
}

func (s *Schema) Version() string {
	if s == nil {
		return ""
	}
	return s.config.Version
}

func (s *Schema) Files() []string {
	return s.config.Files
}

func (s *Schema) Dirs() []string {
	return s.config.Directories
}

func (s *Schema) Excludes() []string {
	return s.config.Excludes
}

func (s *Schema) Walk(e *yang.Entry, fn func(ec *yang.Entry) error) error {
	if e == nil {
		e = s.root
	}
	var err error
	if e.IsCase() || e.IsChoice() {
		for _, ee := range e.Dir {
			err = s.Walk(ee, fn)
			if err != nil {
				return err
			}
		}
		return nil
	}
	err = fn(e)
	if err != nil {
		return err
	}
	for _, e := range e.Dir {
		err = s.Walk(e, fn)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Schema) Reset() {
	s.modules = nil
	s.root = nil
}
