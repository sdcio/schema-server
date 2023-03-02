package target

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/iptecharch/schema-server/config"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/scrapli/scrapligo/driver/netconf"
	"github.com/scrapli/scrapligo/driver/options"
	"github.com/scrapli/scrapligo/util"
	log "github.com/sirupsen/logrus"
)

type ncTarget struct {
	driver       *netconf.Driver
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
	sbi          *config.SBI
}

func newNCTarget(ctx context.Context, cfg *config.SBI, schemaClient schemapb.SchemaServerClient, schema *schemapb.Schema) (*ncTarget, error) {
	var opts []util.Option

	if cfg.Credentials != nil {
		opts = append(opts,
			options.WithAuthUsername(cfg.Credentials.Username),
			options.WithAuthPassword(cfg.Credentials.Password),
			options.WithTransportType("standard"),
			options.WithPort(830),
		)
	}

	opts = append(opts,
		options.WithAuthNoStrictKey(),
		options.WithNetconfForceSelfClosingTags(),
	)

	// init the netconf driver
	d, err := netconf.NewDriver(cfg.Address, opts...)
	if err != nil {
		return nil, err
	}

	err = d.Open()
	if err != nil {
		return nil, err
	}

	return &ncTarget{
		driver:       d,
		schemaClient: schemaClient,
		sbi:          cfg,
		schema:       schema,
	}, nil
}

func (t *ncTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {

	source := "running"

	xmlc := newXMLConfigBuilder(t.schemaClient, t.schema, false)
	for _, p := range req.Path {
		_, err := xmlc.AddElement(ctx, p)
		if err != nil {
			return nil, err
		}
	}

	filterDoc, err := xmlc.GetDoc()
	if err != nil {
		return nil, err
	}
	fmt.Println(filterDoc)
	filter := CreateFilterOption(filterDoc)
	ncResponse, err := t.driver.GetConfig(source, filter, options.WithNetconfForceSelfClosingTags())
	if err != nil {
		return nil, err
	}
	if ncResponse.Failed != nil {
		return nil, ncResponse.Failed
	}

	fmt.Println(ncResponse.Result)

	data := NewXML2SchemapbConfigAdapter(t.schemaClient, t.schema)

	err = data.ParseXMLConfig(ncResponse.Result, "/rpc-reply/data/*")
	if err != nil {
		return nil, err
	}

	fmt.Println()
	fmt.Println()
	fmt.Println()
	fmt.Println()
	fmt.Println("-------------------------------")

	data.Transform(ctx)

	return nil, nil

}

type XML2SchemapbConfigAdapter struct {
	doc          *etree.Document
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
}

func (x *XML2SchemapbConfigAdapter) ParseXMLConfig(doc string, newRootXpath string) error {
	err := x.doc.ReadFromString(doc)
	if err != nil {
		return nil
	}
	r := x.doc.FindElement(newRootXpath)
	if r == nil {
		return fmt.Errorf("unable to find %q in %s", newRootXpath, doc)
	}
	x.doc.SetRoot(r)
	return nil
}

func (x *XML2SchemapbConfigAdapter) Transform(ctx context.Context) {
	result := &schemapb.Notification{}
	x.transformRecursive(ctx, x.doc.Root(), []*schemapb.PathElem{}, result, nil)
}

func (x *XML2SchemapbConfigAdapter) transformRecursive(ctx context.Context, e *etree.Element, pelems []*schemapb.PathElem, result *schemapb.Notification, tc *TransformationContext) error {
	pelems = append(pelems, &schemapb.PathElem{Name: e.Tag})

	// retrieve schema
	sr, err := x.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
		Path: &schemapb.Path{
			Elem: pelems,
		},
		Schema: x.schema,
	})
	if err != nil {
		return err
	}

	switch sr.Schema.(type) {
	case *schemapb.GetSchemaResponse_Container:
		err = x.transformContainer(ctx, e, sr, pelems, result)
		if err != nil {
			return err
		}

	case *schemapb.GetSchemaResponse_Field:
		err = x.transformField(ctx, e, pelems, result)
		if err != nil {
			return err
		}

	case *schemapb.GetSchemaResponse_Leaflist:
		err = x.transformLeafList(ctx, e, sr, pelems, result, tc)
		if err != nil {
			return err
		}
	}

	return nil
}

func (x *XML2SchemapbConfigAdapter) transformContainer(ctx context.Context, e *etree.Element, sr *schemapb.GetSchemaResponse, pelems []*schemapb.PathElem, result *schemapb.Notification) error {
	c := sr.GetContainer()
	for _, ls := range c.Keys {
		pelem := pelems[len(pelems)-1]
		if pelem.Key == nil {
			pelem.Key = map[string]string{}
		}
		pelem.Key[ls.Name] = e.FindElement("./" + ls.Name).Text()
	}

	ntc := NewTransformationContext(pelems)

	// continue with all child
	for _, ce := range e.ChildElements() {
		err := x.transformRecursive(ctx, ce, pelems, result, ntc)
		if err != nil {
			return err
		}
	}

	leafListUpdates := ntc.Close()
	result.Update = append(result.Update, leafListUpdates...)

	return nil
}

func (x *XML2SchemapbConfigAdapter) transformLeafList(ctx context.Context, e *etree.Element, sr *schemapb.GetSchemaResponse, pelems []*schemapb.PathElem, result *schemapb.Notification, tc *TransformationContext) error {

	// process terminal values
	data := strings.TrimSpace(e.Text())

	typedval := &schemapb.TypedValue{Value: &schemapb.TypedValue_StringVal{StringVal: data}}

	name := pelems[len(pelems)-1].Name
	tc.AddLeafListEntry(name, typedval)
	return nil
}

type TransformationContext struct {
	// LeafList contains the leaflist of the actual hierarchie level
	// it is to be converted into an schemapb.update on existing the level
	leafLists map[string][]*schemapb.TypedValue
	pelems    []*schemapb.PathElem
}

func NewTransformationContext(pelems []*schemapb.PathElem) *TransformationContext {
	return &TransformationContext{
		pelems: pelems,
	}
}

// Close when closing the context, updates for LeafLists will be calculated and returned
func (tc *TransformationContext) Close() []*schemapb.Update {
	result := []*schemapb.Update{}
	// process LeafList Elements
	for k, v := range tc.leafLists {

		pathElems := append(tc.pelems, &schemapb.PathElem{Name: k})

		u := &schemapb.Update{
			Path: &schemapb.Path{
				Elem: pathElems,
			},
			Value: &schemapb.TypedValue{
				Value: &schemapb.TypedValue_LeaflistVal{
					LeaflistVal: &schemapb.ScalarArray{
						Element: v,
					},
				},
			},
		}
		fmt.Println(u)
		result = append(result, u)
	}
	return result
}

func (tc *TransformationContext) String() (result string, err error) {
	result = "TransformationContext\n"
	for k, v := range tc.leafLists {
		vals := []string{}
		_ = vals
		for _, val := range v {
			sval, err := valueAsString(val)
			if err != nil {
				return "", err
			}
			vals = append(vals, sval)
		}
		result += fmt.Sprintf("k: %s [%s]\n", k, strings.Join(vals, ", "))
	}
	return result, nil
}

func (tc *TransformationContext) AddLeafListEntry(name string, val *schemapb.TypedValue) error {

	// we do not expect the leafLists to be excesively in use, so we do late initialization
	// although the check is performed on every call to this function
	if tc.leafLists == nil {
		tc.leafLists = map[string][]*schemapb.TypedValue{}
	}

	var exists bool
	// add the tv array if it is the first element and hence does not exist
	if _, exists = tc.leafLists[name]; !exists {
		tc.leafLists[name] = []*schemapb.TypedValue{}
	}
	// append the tv to the list
	tc.leafLists[name] = append(tc.leafLists[name], val)
	return nil
}

func (x *XML2SchemapbConfigAdapter) transformField(ctx context.Context, e *etree.Element, pelems []*schemapb.PathElem, result *schemapb.Notification) error {
	// process terminal values
	data := strings.TrimSpace(e.Text())

	// create schemapb.update
	u := &schemapb.Update{
		Path: &schemapb.Path{
			Elem: pelems,
		},
		Value: &schemapb.TypedValue{Value: &schemapb.TypedValue_StringVal{StringVal: data}},
	}
	result.Update = append(result.Update, u)
	fmt.Println(u)
	return nil
}

func NewXML2SchemapbConfigAdapter(ssc schemapb.SchemaServerClient, schema *schemapb.Schema) *XML2SchemapbConfigAdapter {
	return &XML2SchemapbConfigAdapter{
		doc:          etree.NewDocument(),
		schemaClient: ssc,
		schema:       schema,
	}
}

func CreateFilterOption(filter string) util.Option {
	return func(x interface{}) error {
		oo, ok := x.(*netconf.OperationOptions)

		if !ok {
			return util.ErrIgnoredOption
		}
		oo.Filter = filter
		return nil
	}
}

func (t *ncTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {

	xmlc := newXMLConfigBuilder(t.schemaClient, t.schema, false)

	for _, u := range req.Update {
		xmlc.Add(ctx, u.Path, u.Value)
	}
	// TODO: take care on interferrance of Delete vs. Update

	for _, d := range req.Delete {
		xmlc.Delete(ctx, d)
	}

	xdoc, err := xmlc.GetDoc()
	if err != nil {
		return nil, err
	}

	// add extra <config></config> element
	xdoc = fmt.Sprintf("<config>%s</config>", xdoc)
	fmt.Println(xdoc)

	// edit the config
	resp, err := t.driver.EditConfig("candidate", xdoc)
	if err != nil {
		return nil, err
	}
	if resp.Failed != nil {
		return nil, resp.Failed
	}

	// commit the config
	resp, err = t.driver.Commit()
	if err != nil {
		return nil, err
	}
	if resp.Failed != nil {
		return nil, resp.Failed
	}

	return nil, nil
}

func (t *ncTarget) Subscribe() {}
func (t *ncTarget) Sync(ctx context.Context, syncCh chan *schemapb.Notification) {
	log.Infof("starting target %s sync", t.sbi.Address)
	log.Infof("sync still is a NOOP on netconf targets")
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *ncTarget) Close() {
	t.driver.Close()
}

type XMLConfigBuilder struct {
	doc            *etree.Document
	schemaClient   schemapb.SchemaServerClient
	schema         *schemapb.Schema
	honorNamespace bool
}

func newXMLConfigBuilder(ssc schemapb.SchemaServerClient, schema *schemapb.Schema, honorNamespace bool) *XMLConfigBuilder {
	return &XMLConfigBuilder{
		doc:            etree.NewDocument(),
		schemaClient:   ssc,
		schema:         schema,
		honorNamespace: honorNamespace,
	}
}

func (x *XMLConfigBuilder) GetDoc() (string, error) {
	//x.addNamespaceDefs()
	x.doc.Indent(2)
	xdoc, err := x.doc.WriteToString()
	if err != nil {
		return "", err
	}
	return xdoc, nil
}

func (x *XMLConfigBuilder) Delete(ctx context.Context, p *schemapb.Path) error {

	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return err
	}
	// add the delete operation
	elem.CreateAttr("operation", "delete")

	return nil
}

func (x *XMLConfigBuilder) fastForward(ctx context.Context, p *schemapb.Path) (*etree.Element, error) {
	elem := &x.doc.Element

	actualNamespace := ""

	for peIdx, pe := range p.Elem {

		//namespace := x.namespaces.Resolve(namespaceUri)

		// generate an xpath from the path element
		// this is to find the next level xml element
		path, err := pathElem2Xpath(pe, "")
		if err != nil {
			return nil, err
		}
		var nextElem *etree.Element
		if nextElem = elem.FindElementPath(path); nextElem == nil {

			namespaceUri, err := x.ResolveNamespace(ctx, p, peIdx)
			if err != nil {
				return nil, err
			}

			// if there is no such element, create it
			//elemName := toNamespacedName(pe.Name, namespace)
			nextElem = elem.CreateElement(pe.Name)
			if x.honorNamespace && namespaceUri != actualNamespace {
				nextElem.CreateAttr("xmlns", namespaceUri)
			}
			// with all its keys
			for k, v := range pe.Key {
				//keyNamespaced := toNamespacedName(k, namespace)
				keyElem := nextElem.CreateElement(k)
				keyElem.CreateText(v)
			}
		}
		// prepare next iteration
		elem = nextElem
		xmlns := elem.SelectAttrValue("xmlns", "")
		if xmlns != "" {
			actualNamespace = elem.Space
		}
	}
	return elem, nil
}

func (x *XMLConfigBuilder) Add(ctx context.Context, p *schemapb.Path, v *schemapb.TypedValue) error {

	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return err
	}

	value, err := valueAsString(v)
	if err != nil {
		return err
	}
	elem.CreateText(value)

	return nil
}

func (x *XMLConfigBuilder) AddElement(ctx context.Context, p *schemapb.Path) (*etree.Element, error) {
	elem, err := x.fastForward(ctx, p)
	if err != nil {
		return nil, err
	}
	return elem, nil
}

func (x *XMLConfigBuilder) ResolveNamespace(ctx context.Context, p *schemapb.Path, peIdx int) (namespaceUri string, err error) {
	// Perform schema queries
	sr, err := x.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
		Path: &schemapb.Path{
			Elem:   p.Elem[:peIdx+1],
			Origin: p.Origin,
			Target: p.Target,
		},
		Schema: x.schema,
	})
	if err != nil {
		return "", err
	}

	// deduce namespace from SchemaRequest
	return getNamespaceFromGetSchemaResponse(sr), nil
}

// func (x *XMLConfigBuilder) addNamespaceDefs() {
// 	for uri, nsid := range x.namespaces.GetNamespaces() {
// 		key := fmt.Sprintf("xmlns:%s", nsid)
// 		x.doc.Root().CreateAttr(key, uri)
// 	}
// }

func getNamespaceFromGetSchemaResponse(sr *schemapb.GetSchemaResponse) (namespace string) {
	switch sr.Schema.(type) {
	case *schemapb.GetSchemaResponse_Container:
		namespace = sr.GetContainer().Namespace
	case *schemapb.GetSchemaResponse_Field:
		namespace = sr.GetField().Namespace
	case *schemapb.GetSchemaResponse_Leaflist:
		namespace = sr.GetLeaflist().Namespace
	}
	return namespace
}

func valueAsString(v *schemapb.TypedValue) (string, error) {

	switch v.Value.(type) {
	case *schemapb.TypedValue_StringVal:
		return v.GetStringVal(), nil
	case *schemapb.TypedValue_IntVal:
		return fmt.Sprintf("%d", v.GetIntVal()), nil
	case *schemapb.TypedValue_UintVal:
		return fmt.Sprintf("%d", v.GetUintVal()), nil
	case *schemapb.TypedValue_BoolVal:
		return string(strconv.FormatBool(v.GetBoolVal())), nil
	case *schemapb.TypedValue_BytesVal:
		return string(v.GetBytesVal()), nil
	case *schemapb.TypedValue_FloatVal:
		return string(strconv.FormatFloat(float64(v.GetFloatVal()), 'b', -1, 32)), nil
	case *schemapb.TypedValue_DecimalVal:
		return fmt.Sprintf("%d", v.GetDecimalVal().Digits), nil
	case *schemapb.TypedValue_AsciiVal:
		return v.GetAsciiVal(), nil
	case *schemapb.TypedValue_LeaflistVal:
	case *schemapb.TypedValue_AnyVal:
	case *schemapb.TypedValue_JsonVal:
	case *schemapb.TypedValue_JsonIetfVal:
	case *schemapb.TypedValue_ProtoBytes:
	}
	return "", fmt.Errorf("TypedValue to String failed")
}

func pathElem2Xpath(pe *schemapb.PathElem, namespace string) (etree.Path, error) {
	var keys []string

	// prepare the keys -> "k=v"
	for k, v := range pe.Key {
		keys = append(keys, fmt.Sprintf("%s='%s'", k, v))
	}

	keyString := ""
	if len(keys) > 0 {
		// join multiple key elements via comma
		keyString = "[" + strings.Join(keys, ",") + "]"
	}

	name := pe.Name
	//name := toNamespacedName(pe.Name, namespace)

	// build the final xpath
	filterString := fmt.Sprintf("./%s%s", name, keyString)
	return etree.CompilePath(filterString)
}

// func toNamespacedName(name, namespace string) string {
// 	if namespace != "" {
// 		name = fmt.Sprintf("%s:%s", namespace, name)
// 	}
// 	return name
// }

// type NamespaceIndex struct {
// 	namespaces map[string]string   // namespace uri -> key
// 	keys       map[string]struct{} // map of already assigned keys
// }

// func NewNamespaceIndex() *NamespaceIndex {
// 	return &NamespaceIndex{
// 		namespaces: map[string]string{},
// 		keys:       map[string]struct{}{},
// 	}
// }

// // GetNamespaces returs the namespace uri and keys
// // the format is uri -> key
// func (ni *NamespaceIndex) GetNamespaces() map[string]string {
// 	return ni.namespaces
// }

// func (ni *NamespaceIndex) Resolve(uri string) (identifier string) {
// 	if uri == "" {
// 		return ""
// 	}
// 	if id, exists := ni.namespaces[uri]; exists {
// 		return id
// 	}
// 	return ni.addNewEntry(uri)
// }

// // getNextKey determines the next unused identifer for the namespaces
// func (ni *NamespaceIndex) getNextKey() (key string) {
// 	length := len(ni.keys) + 1
// 	for {
// 		key := intToLetters(length)
// 		if _, exists := ni.keys[key]; !exists {
// 			return key
// 		}
// 		length = length + 1
// 	}
// }

// // addNewEntry adds a new uri to the NamespaceIndex
// // using getNextKey to determine which key to assign
// func (ni *NamespaceIndex) addNewEntry(uri string) (identifier string) {
// 	k := ni.getNextKey()
// 	ni.keys[k] = struct{}{}
// 	ni.namespaces[uri] = k
// 	return k
// }

// // intToLetters converts a number into Alpabetical characters
// // 0...25 => A...Z
// // 26... => AA...AZ
// // ...
// func intToLetters(number int) (letters string) {
// 	number--
// 	if firstLetter := number / 26; firstLetter > 0 {
// 		letters += intToLetters(firstLetter)
// 		letters += string('A' + number%26)
// 	} else {
// 		letters += string('A' + number)
// 	}
// 	return
// }
