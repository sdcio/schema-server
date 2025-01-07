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

package persiststore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/jellydator/ttlcache/v3"
	"github.com/openconfig/goyang/pkg/yang"
	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/sdcio/schema-server/pkg/config"
	"github.com/sdcio/schema-server/pkg/schema"
	"github.com/sdcio/schema-server/pkg/store"
	"github.com/sdcio/schema-server/pkg/utils"
)

const (
	schemasPrefix       uint8 = 1
	schemaObjectsPrefix uint8 = 2
	//
	schemaNameSep = "@"
)

var (
	schemaNameSepByte = []byte(schemaNameSep)
	ErrKeyNotFound    = errors.New("key not found")
)

type cacheKey struct {
	store.SchemaKey
	Path string
}

type persistStore struct {
	path                 string
	cacheWithDescription bool
	cfn                  context.CancelFunc
	db                   *badger.DB
	cache                *ttlcache.Cache[cacheKey, *sdcpb.GetSchemaResponse]
}

func New(ctx context.Context, p string, cfg *config.SchemaPersistStoreCacheConfig) (store.Store, error) {
	s := &persistStore{path: p}
	var err error
	s.db, err = s.openDB(ctx)
	if err != nil {
		return nil, err
	}
	// without cache
	if cfg == nil {
		return s, nil
	}
	// with cache
	s.cacheWithDescription = cfg.WithDescription
	s.cache = ttlcache.New[cacheKey, *sdcpb.GetSchemaResponse](
		ttlcache.WithTTL[cacheKey, *sdcpb.GetSchemaResponse](cfg.TTL),
		ttlcache.WithCapacity[cacheKey, *sdcpb.GetSchemaResponse](cfg.Capacity),
	)
	// start cache cleanup
	go s.cache.Start()
	return s, nil
}

func (s *persistStore) GetSchema(ctx context.Context, req *sdcpb.GetSchemaRequest) (*sdcpb.GetSchemaResponse, error) {
	sck := store.SchemaKey{
		Name:    req.GetSchema().GetName(),
		Vendor:  req.GetSchema().GetVendor(),
		Version: req.GetSchema().GetVersion(),
	}
	if !s.HasSchema(sck) {
		return nil, fmt.Errorf("unknown schema %v", req.GetSchema())
	}
	return s.getSchema(ctx, req, sck)
}

func (s *persistStore) HasSchema(sck store.SchemaKey) bool {
	k := buildSchemaKey(sck)
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(k)
		if err != nil {
			return err
		}
		if item == nil {
			return ErrKeyNotFound
		}
		return nil
	})
	return err == nil
}

func (s *persistStore) ListSchema(ctx context.Context, req *sdcpb.ListSchemaRequest) (*sdcpb.ListSchemaResponse, error) {
	rs := &sdcpb.ListSchemaResponse{
		Schema: []*sdcpb.Schema{},
	}
	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek([]byte{schemasPrefix}); it.ValidForPrefix([]byte{schemasPrefix}); it.Next() {
			item := it.Item()
			parts := bytes.SplitN(item.Key()[1:], schemaNameSepByte, 3)
			if len(parts) != 3 {
				log.Errorf("unexpected schema key format: %s", item.Key())
				continue
			}
			schema := &sdcpb.Schema{
				Name:    string(parts[0]),
				Vendor:  string(parts[1]),
				Version: string(parts[2]),
			}
			rs.Schema = append(rs.Schema, schema)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (s *persistStore) GetSchemaDetails(ctx context.Context, req *sdcpb.GetSchemaDetailsRequest) (*sdcpb.GetSchemaDetailsResponse, error) {
	rs := &sdcpb.GetSchemaDetailsResponse{
		Schema:    req.GetSchema(),
		File:      []string{},
		Directory: []string{},
		Exclude:   []string{},
	}
	err := s.db.View(func(txn *badger.Txn) error {
		k := buildSchemaKey(store.SchemaKey{
			Name:    req.GetSchema().GetName(),
			Vendor:  req.GetSchema().GetVendor(),
			Version: req.GetSchema().GetVersion(),
		})
		item, err := txn.Get(k)
		if err != nil {
			return err
		}
		val, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		cfg := map[string][]string{}
		err = json.Unmarshal(val, &cfg)
		if err != nil {
			return err
		}
		rs.File = cfg["files"]
		rs.Directory = cfg["directories"]
		rs.Exclude = cfg["excludes"]
		return nil
	})
	if err != nil {
		return nil, err
	}
	return rs, nil
}

func (s *persistStore) CreateSchema(ctx context.Context, req *sdcpb.CreateSchemaRequest) (*sdcpb.CreateSchemaResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}
	// fail if the schema already exists
	if s.HasSchema(store.SchemaKey{
		Name:    req.GetSchema().GetName(),
		Vendor:  req.GetSchema().GetVendor(),
		Version: req.GetSchema().GetVersion(),
	}) {
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
	now := time.Now()
	err = s.AddSchema(sc)
	if err != nil {
		return nil, err
	}
	log.Infof("schema %s saved in %s", sc.UniqueName(""), time.Since(now))
	return &sdcpb.CreateSchemaResponse{
		Schema: reqSchema,
	}, nil
}

func (s *persistStore) ReloadSchema(ctx context.Context, req *sdcpb.ReloadSchemaRequest) (*sdcpb.ReloadSchemaResponse, error) {
	details, err := s.GetSchemaDetails(ctx, &sdcpb.GetSchemaDetailsRequest{
		Schema: req.GetSchema(),
	})
	if err != nil {
		return nil, err
	}
	// parse
	sc, err := schema.NewSchema(
		&config.SchemaConfig{
			Name:        req.GetSchema().GetName(),
			Vendor:      req.GetSchema().GetVendor(),
			Version:     req.GetSchema().GetVersion(),
			Files:       details.GetFile(),
			Directories: details.GetDirectory(),
			Excludes:    details.GetExclude(),
		},
	)
	if err != nil {
		return nil, err
	}
	_, err = s.DeleteSchema(ctx, &sdcpb.DeleteSchemaRequest{
		Schema: req.GetSchema(),
	})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	err = s.AddSchema(sc)
	if err != nil {
		return nil, err
	}
	log.Infof("schema %s saved in %s", sc.UniqueName(""), time.Since(now))
	return &sdcpb.ReloadSchemaResponse{}, nil
}

func (s *persistStore) DeleteSchema(ctx context.Context, req *sdcpb.DeleteSchemaRequest) (*sdcpb.DeleteSchemaResponse, error) {
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
	if !s.HasSchema(schemaKey) {
		return nil, status.Errorf(codes.InvalidArgument, "schema %v does not exist", reqSchema)
	}
	// schemaObjectsPrefix [1]$Name@$Vendor@$Version:::
	schemaObjectsPrefix := buildEntryKey(schemaKey, []string{""})
	// schemaPrefix [0]$Name@$Vendor@$Version
	schemaPrefix := buildSchemaKey(schemaKey)
	err := s.db.DropPrefix(schemaObjectsPrefix, schemaPrefix)
	if err != nil {
		return nil, err
	}
	return &sdcpb.DeleteSchemaResponse{}, nil
}

func (s *persistStore) AddSchema(sc *schema.Schema) error {
	sck := store.Key(sc)
	// TODO: delete all
	e, err := sc.GetEntry(nil)
	if err != nil {
		return err
	}

	wb := s.db.NewWriteBatch()
	defer wb.Cancel()

	err = s.addSchemaElem(wb, sc, e)
	if err != nil {
		return err
	}
	err = s.addSchema(wb, sck,
		map[string][]string{
			"files":       sc.Files(),
			"directories": sc.Dirs(),
			"excludes":    sc.Excludes(),
		})
	if err != nil {
		return err
	}

	err = wb.Flush()
	if err != nil {
		return err
	}
	sc.Reset()
	return nil
}

func (s *persistStore) GetSchemaElements(ctx context.Context, req *sdcpb.GetSchemaRequest) (chan *sdcpb.SchemaElem, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}

	sck := store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}
	if !s.HasSchema(sck) {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	// verify the path exists in the schema
	rsp, err := s.getSchema(ctx, req, sck)
	if err != nil {
		return nil, err
	}
	sch := make(chan *sdcpb.SchemaElem, 1)
	// if the path has a single element,
	// that's the schema elem we used to verify the
	// path validity.
	// send it and close the channel
	if len(req.GetPath().GetElem()) == 1 {
		sch <- rsp.GetSchema()
		close(sch)
		return sch, nil
	}

	go func() {
		defer close(sch)
		for i := 0; i < len(req.GetPath().GetElem()); i++ {
			sp := &sdcpb.Path{Elem: make([]*sdcpb.PathElem, 0, i+1)}
			for j := 0; j < i+1; j++ {
				sp.Elem = append(sp.Elem, &sdcpb.PathElem{Name: req.GetPath().GetElem()[j].GetName()})
			}
			rsp, err := s.getSchema(ctx, &sdcpb.GetSchemaRequest{
				Path:            sp,
				Schema:          reqSchema,
				ValidateKeys:    req.GetValidateKeys(),
				WithDescription: req.GetWithDescription(),
			}, sck)
			if err != nil {
				log.Errorf("failed getting entries from schema: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case sch <- rsp.GetSchema():
			}
		}
	}()

	return sch, nil
}

func (s *persistStore) ToPath(ctx context.Context, req *sdcpb.ToPathRequest) (*sdcpb.ToPathResponse, error) {
	reqSchema := req.GetSchema()
	if reqSchema == nil {
		return nil, status.Error(codes.InvalidArgument, "missing schema details")
	}

	sck := store.SchemaKey{Name: reqSchema.Name, Vendor: reqSchema.Vendor, Version: reqSchema.Version}
	if !s.HasSchema(sck) {
		return nil, status.Errorf(codes.InvalidArgument, "unknown schema %v", reqSchema)
	}
	numPathElems := len(req.GetPathElement())
	p := &sdcpb.Path{
		Elem: make([]*sdcpb.PathElem, 0, numPathElems),
	}
	i := 0
OUTER:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			if i >= numPathElems {
				break OUTER
			}
			p.Elem = append(p.Elem, &sdcpb.PathElem{Name: req.PathElement[i]})
			if i >= numPathElems-1 {
				break OUTER
			}
			rsp, err := s.getSchema(ctx, &sdcpb.GetSchemaRequest{
				Path:            p,
				Schema:          req.GetSchema(),
				ValidateKeys:    false,
				WithDescription: false,
			}, sck)
			if err != nil {
				return nil, err
			}

			switch rsp.GetSchema().Schema.(type) {
			case *sdcpb.SchemaElem_Container:
				p.Elem[len(p.GetElem())-1].Key = make(map[string]string, len(rsp.GetSchema().GetContainer().GetKeys()))
				// assumes keys are sorted by name
				for _, schemaKey := range rsp.GetSchema().GetContainer().GetKeys() {
					p.Elem[len(p.GetElem())-1].Key[schemaKey.GetName()] = req.GetPathElement()[i+1]
					i++
					if i >= numPathElems {
						break OUTER
					}
				}
			case *sdcpb.SchemaElem_Field:
			case *sdcpb.SchemaElem_Leaflist:
				name := p.Elem[len(p.GetElem())-1].GetName()
				if p.Elem[len(p.GetElem())-1].Key == nil {
					p.Elem[len(p.GetElem())-1].Key = make(map[string]string)
				}
				p.Elem[len(p.GetElem())-1].Key[name] = req.PathElement[i+1]
				i++
			}
			//
			i++
		}
	}
	if numPathElems-1 > i {
		return nil, fmt.Errorf("unknown PathElement(s): %s", req.GetPathElement()[i:])
	}
	// validate final path
	_, err := s.getSchema(ctx, &sdcpb.GetSchemaRequest{
		Path:            p,
		Schema:          req.GetSchema(),
		ValidateKeys:    false,
		WithDescription: false,
	}, sck)
	if err != nil {
		return nil, err
	}
	return &sdcpb.ToPathResponse{Path: p}, nil
}

func (s *persistStore) ExpandPath(ctx context.Context, req *sdcpb.ExpandPathRequest) (*sdcpb.ExpandPathResponse, error) {
	p := req.GetPath()
	// does the path exist ?
	rsp, err := s.GetSchema(ctx, &sdcpb.GetSchemaRequest{
		Path:   p,
		Schema: req.GetSchema(),
	})
	if err != nil {
		return nil, err
	}
	// final response
	pRsp := &sdcpb.ExpandPathResponse{
		Path:  []*sdcpb.Path{},
		Xpath: []string{},
	}

	switch rsp := rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		pp := proto.Clone(p).(*sdcpb.Path)
		if pp.Elem == nil {
			pp.Elem = make([]*sdcpb.PathElem, 0, 1)
		}
		// add keys to the last path element
		for _, key := range rsp.Container.GetKeys() {
			// pp = proto.Clone(pp).(*sdcpb.Path)
			if pp.GetElem()[len(pp.GetElem())-1].GetKey() == nil {
				pp.Elem[len(pp.GetElem())-1].Key = make(map[string]string)
			}
			pp.Elem[len(pp.GetElem())-1].Key[key.Name] = "*"
			// DO NOT ADD paths with keys as leaves (this can be done client side)
		}
		// add fields (YANG leaf)
		for _, field := range rsp.Container.GetFields() {
			pp := proto.Clone(pp).(*sdcpb.Path)
			if pp.Elem == nil {
				pp.Elem = make([]*sdcpb.PathElem, 0, 1)
			}
			pp.Elem = append(pp.Elem, &sdcpb.PathElem{Name: field.Name})
			addPath(pRsp, pp, req.GetDataType(), field.IsState, req.GetXpath())
		}
		// add leaf-lists
		for _, lf := range rsp.Container.GetLeaflists() {
			pp := proto.Clone(pp).(*sdcpb.Path)
			if pp.Elem == nil {
				pp.Elem = make([]*sdcpb.PathElem, 0, 1)
			}
			pp.Elem = append(pp.Elem, &sdcpb.PathElem{Name: lf.Name})
			addPath(pRsp, pp, req.GetDataType(), lf.IsState, req.GetXpath())
		}
		// add containers(YANG container, list, choice, case,...)
		for _, child := range rsp.Container.GetChildren() {
			pp := proto.Clone(pp).(*sdcpb.Path)
			if pp.Elem == nil {
				pp.Elem = make([]*sdcpb.PathElem, 0, 1)
			}
			pp.Elem = append(pp.Elem, &sdcpb.PathElem{Name: child})
			expRsp, err := s.ExpandPath(ctx, &sdcpb.ExpandPathRequest{
				Path:     pp,
				Schema:   req.GetSchema(),
				DataType: req.GetDataType(),
				Xpath:    req.GetXpath(),
			})
			if err != nil {
				return nil, err
			}
			pRsp.Path = append(pRsp.Path, expRsp.Path...)
			pRsp.Xpath = append(pRsp.Xpath, expRsp.Xpath...)
		}
	case *sdcpb.SchemaElem_Field:
		addPath(pRsp, p, req.GetDataType(), rsp.Field.IsState, req.GetXpath())
	case *sdcpb.SchemaElem_Leaflist:
		addPath(pRsp, p, req.GetDataType(), rsp.Leaflist.IsState, req.GetXpath())
	}
	return pRsp, nil
}

// addPath adds path p to response rsp based on the requested dataType and the schema object isState value
func addPath(rsp *sdcpb.ExpandPathResponse, p *sdcpb.Path, dt sdcpb.DataType, isState, xpath bool) {
	switch dt {
	case sdcpb.DataType_ALL:
	case sdcpb.DataType_CONFIG:
		if isState {
			return
		}
	case sdcpb.DataType_STATE:
		if !isState {
			return
		}
	}
	if xpath {
		rsp.Xpath = append(
			rsp.Xpath,
			utils.ToXPath(p, false),
		)
		return
	}
	rsp.Path = append(rsp.Path, p)
}

// helpers
func (s *persistStore) addSchemaElem(wb *badger.WriteBatch, sc *schema.Schema, e *yang.Entry) error {
	// skip choice and case from paths
	if e.IsChoice() || e.IsCase() {
		for _, ee := range e.Dir {
			err := s.addSchemaElem(wb, sc, ee)
			if err != nil {
				return err
			}
		}
		return nil
	}
	// build entry key
	key := buildEntryKey(store.Key(sc), getEntryPath(e))
	// get entry proto
	se, err := sc.SchemaElemFromYEntry(e, true)
	if err != nil {
		return err
	}
	b, err := proto.Marshal(se)
	if err != nil {
		return err
	}
	// store entry proto bytes
	err = wb.Set(key, b)
	if err != nil {
		return err
	}
	// do the same for entry children
	for _, ee := range e.Dir {
		err = s.addSchemaElem(wb, sc, ee)
		if err != nil {
			return err
		}
	}
	return nil
}

// save schema name with prefix 1
func (s *persistStore) addSchema(wb *badger.WriteBatch, sck store.SchemaKey, cfg map[string][]string) error {
	v, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	k := buildSchemaKey(sck)
	return wb.Set(k, v)
}

func buildSchemaKey(sck store.SchemaKey) []byte {
	k := []byte(schemaKeyString(sck))
	k = append(k, 0)
	copy(k[1:], k[:len(k)-1])
	k[0] = schemasPrefix
	return k
}

func buildEntryKey(sc store.SchemaKey, pes []string) []byte {
	sb := &bytes.Buffer{}
	sb.WriteByte(schemaObjectsPrefix)
	sb.WriteString(schemaKeyString(sc))
	sb.WriteString(":::")
	sb.WriteString(pes[0])
	if len(pes) == 1 {
		return sb.Bytes()
	}
	sb.WriteString(":::")
	for _, pe := range pes[1:] {
		sb.WriteString("/" + pe)
	}
	return sb.Bytes()
}

func schemaKeyString(sc store.SchemaKey) string {
	return sc.Name + "@" + sc.Vendor + "@" + sc.Version
}

func getEntryPath(e *yang.Entry) []string {
	rs := make([]string, 0, 1)
	rs = append(rs, e.Name)
	for {
		// done
		if e.Parent == nil {
			break
		}
		// done
		if e.Parent.Name == schema.RootName {
			break
		}
		// skip choice and case
		for e.Parent.IsCase() || e.Parent.IsChoice() {
			e = e.Parent
		}
		rs = append(rs, e.Parent.Name)
		e = e.Parent
	}
	reverse(rs)
	return rs
}

func reverse(arr []string) {
	for i := 0; i < len(arr)/2; i++ {
		arr[i], arr[len(arr)-i-1] = arr[len(arr)-i-1], arr[i]
	}
}

func (s *persistStore) openDB(ctx context.Context) (*badger.DB, error) {
	opts := badger.DefaultOptions(s.path).
		WithLoggingLevel(badger.WARNING).
		WithCompression(options.None).
		WithBlockCacheSize(0)

	ctx, s.cfn = context.WithCancel(ctx)

	bdb, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			again:
				log.Debugf("running GC for %s", s.path)
				err = bdb.RunValueLogGC(0.7)
				if err == nil {
					goto again
				}
				log.Debugf("GC for %s ended with err: %v", s.path, err)
			}
		}
	}()
	return bdb, nil
}

func (s *persistStore) getModules(sc store.SchemaKey) ([]string, error) {
	txn := s.db.NewTransaction(false)
	defer txn.Discard()
	return getModules(txn, sc)
}

func getModules(txn *badger.Txn, sc store.SchemaKey) ([]string, error) {
	k := buildEntryKey(sc, []string{schema.RootName})
	item, err := txn.Get(k)
	if err != nil {
		return nil, err
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		return nil, err
	}
	se := &sdcpb.SchemaElem{}
	err = proto.Unmarshal(val, se)
	if err != nil {
		return nil, err
	}
	return se.GetContainer().GetChildren(), nil
}

func (s *persistStore) getSchema(ctx context.Context, req *sdcpb.GetSchemaRequest, sck store.SchemaKey) (*sdcpb.GetSchemaResponse, error) {
	pes := utils.ToStrings(req.GetPath(), false, true)
	cKey := cacheKey{
		SchemaKey: sck,
		Path:      strings.Join(pes, "/"),
	}
	if s.cache != nil {
		if item := s.cache.Get(cKey, ttlcache.WithDisableTouchOnHit[cacheKey, *sdcpb.GetSchemaResponse]()); item != nil {
			// clone it
			rsp := proto.Clone(item.Value()).(*sdcpb.GetSchemaResponse)
			// apply modifiers
			// if the request does not need the description and the
			// cache stores with description, remove it.
			if !req.GetWithDescription() && s.cacheWithDescription {
				return removeDescription(rsp), nil
			}
			return rsp, nil
		}
	}
	var err error
	sce := new(sdcpb.SchemaElem)

	// key all i.e "root"
	if lpes := len(pes); lpes == 0 || (lpes == 1 && pes[0] == "") {
		err = s.db.View(func(txn *badger.Txn) error {
			k := buildEntryKey(sck, []string{schema.RootName})
			item, err := txn.Get(k)
			if err != nil {
				return err
			}
			if item == nil {
				return ErrKeyNotFound
			}
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			err = proto.Unmarshal(v, sce)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return &sdcpb.GetSchemaResponse{Schema: sce}, nil
	}
	moduleName := ""
	if index := strings.Index(pes[0], ":"); index > 0 {
		moduleName = pes[0][:index]
		pes[0] = pes[0][index+1:]
	}
	var modules []string
	// path has module prefix
	if moduleName != "" {
		modules = []string{moduleName}
	} else {
		// path does not have module prefix
		modules, err = s.getModules(sck)
		if err != nil {
			return nil, err
		}
		sort.Slice(modules, func(i, j int) bool {
			return utils.SortModulesAB(modules[i], modules[j], config.DeprioritizedModules)
		})
	}

	npe := make([]string, 1+len(pes))
	copy(npe[1:], pes)
	err = s.db.View(func(txn *badger.Txn) error {
		for _, module := range modules {
			var k []byte
			if npe[1] == module { // query module name
				k = buildEntryKey(sck, npe[1:])
			} else {
				npe[0] = module
				k = buildEntryKey(sck, npe)
			}
			item, err := txn.Get(k)
			if err != nil {
				continue
			}
			if item == nil {
				continue
			}
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			err = proto.Unmarshal(v, sce)
			if err != nil {
				return err
			}
			return nil
		}
		return fmt.Errorf("%s: %w", req.GetPath(), ErrKeyNotFound)
	})
	if err != nil {
		return nil, err
	}
	rsp := &sdcpb.GetSchemaResponse{Schema: sce}
	if s.cache != nil {
		s.cache.Set(cKey, rsp, ttlcache.DefaultTTL)
	}
	return &sdcpb.GetSchemaResponse{Schema: sce}, nil
}

func removeDescription(rsp *sdcpb.GetSchemaResponse) *sdcpb.GetSchemaResponse {
	if rsp == nil {
		return nil
	}

	switch rsp.GetSchema().Schema.(type) {
	case *sdcpb.SchemaElem_Container:
		rsp.GetSchema().GetContainer().Description = ""
	case *sdcpb.SchemaElem_Field:
		rsp.GetSchema().GetField().Description = ""
	case *sdcpb.SchemaElem_Leaflist:
		rsp.GetSchema().GetLeaflist().Description = ""
	}
	return rsp
}
