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
	driver *netconf.Driver
}

func newNCTarget(ctx context.Context, cfg *config.SBI) (*ncTarget, error) {
	var opts []util.Option

	if cfg.Credentials != nil {
		opts = append(opts,
			options.WithAuthUsername(cfg.Credentials.Username),
			options.WithAuthPassword(cfg.Credentials.Password),
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

	return &ncTarget{}, nil
}

func (t *ncTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	return nil, nil
}
func (t *ncTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {

	xmlc := NewXMLConfig()

	for _, u := range req.Update {
		xmlc.Add(u.Path, u.Value)
	}
	_ = xmlc

	return nil, nil
}

func (t *ncTarget) Subscribe() {}
func (t *ncTarget) Sync(ctx context.Context, syncCh chan *schemapb.Notification) {
	<-ctx.Done()
	log.Infof("sync stopped: %v", ctx.Err())
}

func (t *ncTarget) Close() {}

type XMLConfig struct {
	doc *etree.Document
}

func NewXMLConfig() *XMLConfig {
	return &XMLConfig{
		doc: etree.NewDocument(),
	}
}

func (x *XMLConfig) Add(p *schemapb.Path, v *schemapb.TypedValue) error {
	elem := &x.doc.Element
	for _, pe := range p.Elem {
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
