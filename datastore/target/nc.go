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
	_ = xmlc

	return nil, nil
}

func (t *ncTarget) Subscribe() {}
func (t *ncTarget) Sync(ctx context.Context, syncCh chan *schemapb.Notification) {
	log.Infof("starting target %s sync", t.sbi.Address)
	log.Infof("sync still is a NOOP on netconf targets")
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *ncTarget) Close() {}

type XMLConfigBuilder struct {
	doc          *etree.Document
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
}

func NewXMLConfigBuilder(ssc schemapb.SchemaServerClient, schema *schemapb.Schema) *XMLConfigBuilder {
	return &XMLConfigBuilder{
		doc:          etree.NewDocument(),
		schemaClient: ssc,
		schema:       schema,
	}
}

func (x *XMLConfigBuilder) Add(ctx context.Context, p *schemapb.Path, v *schemapb.TypedValue) error {
	elem := &x.doc.Element

	for peIdx, pe := range p.Elem {

		// Perform schema queries
		sr, err := x.schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
			Path: &schemapb.Path{
				Elem:   p.Elem[:peIdx],
				Origin: p.Origin,
				Target: p.Target,
			},
			Schema: x.schema,
		})
		if err != nil {
			return err
		}
		// deduce namespace from SchemaRequest
		namespace := getNamespaceFromGetSchemaResponse(sr)
		_ = namespace

		// generate an xpath from the path element
		// this is to find the next level xml element
		path, err := pathElem2Xpath(pe)
		if err != nil {
			return err
		}
		var nextElem *etree.Element
		if nextElem = elem.FindElementPath(path); nextElem == nil {
			// if there is no such element, create it
			nextElem = elem.CreateElement(pe.Name)
			// with all its keys
			for k, v := range pe.Key {
				keyElem := nextElem.CreateElement(k)
				keyElem.CreateText(v)
			}
		}
		// prepare next iteration
		elem = nextElem
	}

	value, err := valueAsString(v)
	if err != nil {
		return err
	}
	elem.CreateText(value)

	return nil
}

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

func pathElem2Xpath(pe *schemapb.PathElem) (etree.Path, error) {
	var keys []string

	// prepare the keys -> "k=v"
	for k, v := range pe.Key {
		keys = append(keys, fmt.Sprintf("%s=%s", k, v))
	}

	keyString := ""
	if len(keys) > 0 {
		// join multiple key elements via comma
		keyString = "[" + strings.Join(keys, ",") + "]"
	}

	// build the final xpath
	return etree.CompilePath(fmt.Sprintf("./%s%s", pe.Name, keyString))
}
