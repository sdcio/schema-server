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
	return nil, nil
}
func (t *ncTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {

	xmlc := NewXMLConfigBuilder(t.schemaClient, t.schema)

	for _, u := range req.Update {
		xmlc.Add(ctx, u.Path, u.Value)
	}

	for _, d := range req.Delete {
		xmlc.Delete(ctx, d)
	}

	xdoc, err := xmlc.GetDoc()
	if err != nil {
		return nil, err
	}
	fmt.Println(xdoc)

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
	doc          *etree.Document
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
	//namespaces   *NamespaceIndex
}

func NewXMLConfigBuilder(ssc schemapb.SchemaServerClient, schema *schemapb.Schema) *XMLConfigBuilder {
	return &XMLConfigBuilder{
		doc:          etree.NewDocument(),
		schemaClient: ssc,
		schema:       schema,
		//namespaces:   NewNamespaceIndex(),
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

	elem, err := x.forward(ctx, p)
	if err != nil {
		return err
	}
	// add the delete operation
	elem.CreateAttr("operation", "delete")

	return nil
}

func (x *XMLConfigBuilder) forward(ctx context.Context, p *schemapb.Path) (*etree.Element, error) {
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
			if namespaceUri != actualNamespace {
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

	elem, err := x.forward(ctx, p)
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
