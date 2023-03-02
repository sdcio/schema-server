package netconf

import (
	"context"
	"fmt"
	"strings"

	"github.com/beevik/etree"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

type XML2SchemapbConfigAdapter struct {
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
}

func NewXML2SchemapbConfigAdapter(ssc schemapb.SchemaServerClient, schema *schemapb.Schema) *XML2SchemapbConfigAdapter {
	return &XML2SchemapbConfigAdapter{
		schemaClient: ssc,
		schema:       schema,
	}
}

// func (x *XML2SchemapbConfigAdapter) ParseXMLConfig(doc string, newRootXpath string) error {
// 	err := x.doc.ReadFromString(doc)
// 	if err != nil {
// 		return nil
// 	}
// 	r := x.doc.FindElement(newRootXpath)
// 	if r == nil {
// 		return fmt.Errorf("unable to find %q in %s", newRootXpath, doc)
// 	}
// 	x.doc.SetRoot(r)
// 	return nil
// }

func (x *XML2SchemapbConfigAdapter) Transform(ctx context.Context, doc *etree.Document) *schemapb.Notification {
	result := &schemapb.Notification{}
	x.transformRecursive(ctx, doc.Root(), []*schemapb.PathElem{}, result, nil)
	return result
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

func (x *XML2SchemapbConfigAdapter) transformLeafList(ctx context.Context, e *etree.Element, sr *schemapb.GetSchemaResponse, pelems []*schemapb.PathElem, result *schemapb.Notification, tc *TransformationContext) error {

	// process terminal values
	data := strings.TrimSpace(e.Text())

	typedval := &schemapb.TypedValue{Value: &schemapb.TypedValue_StringVal{StringVal: data}}

	name := pelems[len(pelems)-1].Name
	tc.AddLeafListEntry(name, typedval)
	return nil
}
