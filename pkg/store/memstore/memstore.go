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

package memstore

import (
	"context"
	"sort"
	"sync"

	"github.com/openconfig/goyang/pkg/yang"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store"
	"github.com/sdcio/schema-server/pkg/utils"
)

type memStore struct {
	ms      *sync.RWMutex
	schemas map[store.SchemaKey]*schema.Schema
}

func New() store.Store {
	return &memStore{
		ms:      &sync.RWMutex{},
		schemas: map[store.SchemaKey]*schema.Schema{},
	}
}

func (s *memStore) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	s.ms.RLock()
	defer s.ms.RUnlock()
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	pes := utils.ToStrings(req.GetPath(), false, true)

	sc, ok := s.schemas[store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	e, err := sc.GetEntry(pes)
	if err != nil {
		return nil, err
	}
	schemElem, err := schema.SchemaElemFromYEntry(e, req.GetWithDescription())
	if err != nil {
		return nil, err
	}
	resp := &sdcpb.GetSchemaResponse{
		Schema: schemElem,
	}
	log.Tracef("schema response: %v", resp)
	return resp, nil
}

func (s *memStore) HasSchema(scKey store.SchemaKey) bool {
	s.ms.RLock()
	defer s.ms.RUnlock()
	_, ok := s.schemas[scKey]
	return ok
}

func (s *memStore) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	s.ms.RLock()
	defer s.ms.RUnlock()
	rsp := &sdcpb.ListSchemaResponse{
		Schema: make([]*sdcpb.Schema, 0, len(s.schemas)),
	}
	for _, sc := range s.schemas {
		rsp.Schema = append(rsp.Schema,
			&sdcpb.Schema{
				Name:    sc.Name(),
				Vendor:  sc.Vendor(),
				Version: sc.Version(),
			})
	}
	return rsp, nil
}

func (s *memStore) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	s.ms.RLock()
	defer s.ms.RUnlock()
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	sc, ok := s.schemas[store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	rsp := &sdcpb.GetSchemaDetailsResponse{
		Schema: &sdcpb.Schema{
			Name:    sc.Name(),
			Vendor:  sc.Vendor(),
			Version: sc.Version(),
			Status:  0,
		},
		File:      sc.Files(),
		Directory: sc.Dirs(),
	}
	//
	return rsp, nil
}

func (s *memStore) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	s.ms.RLock()
	_, ok := s.schemas[store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}]
	s.ms.RUnlock()
	if ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v already exists", reqSchema)
	}
	switch {
	// case req.GetSchema().GetName() == "":
	// 	return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	sc, err := schema.NewSchema(
		&config.SchemaConfig{
			Name:        req.GetSchema().GetName(),
			Vendor:      req.GetSchema().GetVendor(),
			Version:     req.GetSchema().GetVersion(),
			Files:       req.GetFile(),
			Directories: req.GetDirectory(),
			Excludes:    req.GetExclude(),
		},
	)
	if err != nil {
		return nil, err
	}

	// write
	s.ms.Lock()
	defer s.ms.Unlock()
	s.schemas[store.Key(sc)] = sc
	scrsp := req.GetSchema()

	return &sdcpb.CreateSchemaResponse{
		Schema: scrsp,
	}, nil
}

func (s *memStore) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	s.ms.RLock()
	sc, ok := s.schemas[store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	nsc, err := sc.Reload()
	if err != nil {
		return nil, err
	}
	s.ms.Lock()
	defer s.ms.Unlock()
	s.schemas[store.Key(nsc)] = nsc
	return &sdcpb.ReloadSchemaResponse{}, nil
}

func (s *memStore) DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	switch {
	// case req.GetSchema().GetName() == "":
	// 	return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	schemaKey := store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}
	s.ms.RLock()
	_, ok := s.schemas[schemaKey]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v does not exist", reqSchema)
	}
	s.ms.Lock()
	defer s.ms.Unlock()
	delete(s.schemas, schemaKey)
	return &sdcpb.DeleteSchemaResponse{}, nil
}

func (s *memStore) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	switch {
	// case req.GetSchema().GetName() == "":
	// 	return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	s.ms.RLock()
	sc, ok := s.schemas[store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v does not exist", reqSchema)
	}
	p := &sdcpb.Path{
		Elem: make([]*sdcpb.PathElem, 0),
	}
	err := sc.BuildPath(req.GetPathElement(), p)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	rsp := &sdcpb.ToPathResponse{
		Path: p,
	}
	return rsp, nil
}

func (s *memStore) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	switch {
	// case req.GetSchema().GetName() == "":
	// 	return nil, status.Error(codes.InvalidArgument, "missing schema name")
	case req.GetSchema().GetVendor() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema vendor")
	case req.GetSchema().GetVersion() == "":
		return nil, status.Error(codes.InvalidArgument, "missing schema version")
	}
	s.ms.RLock()
	sc, ok := s.schemas[store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}]
	s.ms.RUnlock()
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v does not exist", reqSchema)
	}
	paths, err := sc.ExpandPath(req.GetPath(), req.GetDataType())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if req.GetXpath() {
		xpaths := make([]string, 0, len(paths))
		for _, p := range paths {
			xpaths = append(xpaths, utils.ToXPath(p, false))
		}
		sort.Strings(xpaths)
		rsp := &sdcpb.ExpandPathResponse{
			Xpath: xpaths,
		}
		return rsp, nil
	}
	rsp := &sdcpb.ExpandPathResponse{
		Path: paths,
	}
	return rsp, nil
}

func (s *memStore) AddSchema(sc *schema.Schema) error {
	s.ms.Lock()
	defer s.ms.Unlock()
	s.schemas[store.Key(sc)] = sc
	return nil
}

func (s *memStore) GetSchemaElements(ctx context.Context, req *sdcpb.GetSchemaRequest) (chan *sdcpb.SchemaElem, error) {
	s.ms.RLock()
	defer s.ms.RUnlock()
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	sc, ok := s.schemas[store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	pes := utils.ToStrings(req.GetPath(), false, true)

	sch := make(chan *sdcpb.SchemaElem)
	ych := make(chan *yang.Entry)
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ych:
				if !ok {
					return
				}
				schemElem, err := schema.SchemaElemFromYEntry(e, req.GetWithDescription())
				if err != nil {
					log.Errorf("failed getting entries from schema: %v", err)
				}
				sch <- schemElem
			}
		}
	}()
	go func() {
		defer wg.Done()
		err := sc.GetEntryCh(pes, ych)
		if err != nil {
			log.Errorf("failed getting entries from schema: %v", err)
		}
	}()
	go func() {
		wg.Wait()
		defer close(sch)
	}()

	return sch, nil
}
