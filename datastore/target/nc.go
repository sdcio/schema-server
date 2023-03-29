package target

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/iptecharch/schema-server/config"
	"github.com/iptecharch/schema-server/datastore/target/netconf"
	"github.com/iptecharch/schema-server/datastore/target/netconf/driver/scrapligo"
	"github.com/iptecharch/schema-server/datastore/target/netconf/types"
	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
	log "github.com/sirupsen/logrus"
	"google.golang.org/protobuf/encoding/prototext"
)

type ncTarget struct {
	name         string
	driver       netconf.Driver
	schemaClient schemapb.SchemaServerClient
	schema       *schemapb.Schema
	sbi          *config.SBI
}

func newNCTarget(_ context.Context, name string, cfg *config.SBI, schemaClient schemapb.SchemaServerClient, schema *schemapb.Schema) (*ncTarget, error) {

	// create a new
	d, err := scrapligo.NewScrapligoNetconfTarget(cfg)
	if err != nil {
		return nil, err
	}

	return &ncTarget{
		name:         name,
		driver:       d,
		schemaClient: schemaClient,
		schema:       schema,
		sbi:          cfg,
	}, nil
}

func (t *ncTarget) Get(ctx context.Context, req *schemapb.GetDataRequest) (*schemapb.GetDataResponse, error) {
	var source string

	switch req.Datastore.Type {
	case schemapb.Type_MAIN:
		source = "running"
	case schemapb.Type_CANDIDATE:
		source = "candidate"
	}

	// init a new XMLConfigBuilder for the pathfilter
	pathfilterXmlBuilder := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, t.sbi.IncludeNS)

	// add all the requested paths to the document
	for _, p := range req.Path {
		_, err := pathfilterXmlBuilder.AddElement(ctx, p)
		if err != nil {
			return nil, err
		}
	}

	// retrieve the xml filter as string
	filterDoc, err := pathfilterXmlBuilder.GetDoc()
	if err != nil {
		return nil, err
	}
	log.Debugf("netconf filter:\n%s", filterDoc)

	// execute the GetConfig rpc
	ncResponse, err := t.driver.GetConfig(source, filterDoc)
	if err != nil {
		return nil, err
	}

	log.Debugf("netconf response:\n%s", ncResponse.DocAsString())

	// init an XML2SchemapbConfigAdapter used to convert the netconf xml config to a schemapb.Notification
	data := netconf.NewXML2SchemapbConfigAdapter(t.schemaClient, t.schema)

	// start transformation, which yields the schemapb_Notification
	noti := data.Transform(ctx, ncResponse.Doc)

	// building the resulting schemapb.GetDataResponse struct
	result := &schemapb.GetDataResponse{
		Notification: []*schemapb.Notification{
			noti,
		},
	}
	return result, nil
}

func (t *ncTarget) Set(ctx context.Context, req *schemapb.SetDataRequest) (*schemapb.SetDataResponse, error) {

	xmlCBDelete := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, t.sbi.IncludeNS)

	// iterate over the delete array
	for _, d := range req.Delete {
		xmlCBDelete.Delete(ctx, d)
	}

	// iterate over the replace array
	// ATTENTION: This is not implemented intentionally, since it is expected,
	//  	that the datastore will only come up with deletes and updates.
	// 		actual replaces will be resolved to deletes and updates by the datastore
	// 		also replaces would only really make sense with jsonIETF encoding, where
	// 		an entire branch is replaces, on single values this is covered via an
	// 		update.
	//
	// for _, r := range req.Replace {
	// }
	//

	xmlCBAdd := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, t.sbi.IncludeNS)

	// iterate over the update array
	for _, u := range req.Update {
		xmlCBAdd.Add(ctx, u.Path, u.Value)
	}

	// first apply the deletes before the adds
	for _, xml := range []*netconf.XMLConfigBuilder{xmlCBDelete, xmlCBAdd} {
		// finally retrieve the xml config as string
		xdoc, err := xml.GetDoc()
		if err != nil {
			return nil, err
		}

		// if there was no data in the xml document, continue
		if len(xdoc) == 0 {
			continue
		}

		log.Debugf("datastore %s XML:\n%s\n", t.name, xdoc)

		// edit the config
		_, err = t.driver.EditConfig("candidate", xdoc)
		if err != nil {
			log.Errorf("datastore %s failed edit-config: %v", t.name, err)
			err2 := t.driver.Discard()
			if err != nil {
				// log failed discard
				log.Errorf("failed with %v while discarding pending changes after error %v", err2, err)
			}
			return nil, err
		}

	}
	log.Infof("datastore %s: committing changes on target", t.name)
	// commit the config
	err := t.driver.Commit()
	if err != nil {
		return nil, err
	}
	return &schemapb.SetDataResponse{
		Timestamp: time.Now().UnixNano(),
	}, nil
}

func (t *ncTarget) Subscribe() {}

func (t *ncTarget) Sync(ctx context.Context, syncConfig *config.Sync, syncCh chan *SyncUpdate) {
	log.Infof("starting target %s sync", t.sbi.Address)
	wg := new(sync.WaitGroup)
	wg.Add(len(syncConfig.NC))
	for _, ncSync := range syncConfig.NC {
		go func(ncSync *config.NCSync) {
			defer wg.Done()
			xb := netconf.NewXMLConfigBuilder(t.schemaClient, t.schema, true)
			for _, p := range ncSync.Paths {
				sp, err := utils.ParsePath(p)
				if err != nil {
					log.Errorf("failed to parse path %s: %v", p, err)
					return
				}
				_, err = xb.AddElement(ctx, sp)
				if err != nil {
					log.Errorf("failed to add path %s to XML doc: %v", p, err)
					return
				}
			}
			filter, err := xb.GetDoc()
			if err != nil {
				log.Errorf("failed to build XML filter: %v", err)
				return
			}
			ticker := time.NewTicker(ncSync.Period)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					var rsp *types.NetconfResponse
					var err error
					switch ncSync.Name {
					case "config":
						rsp, err = t.driver.GetConfig("running", filter)
					case "state":
						rsp, err = t.driver.Get(filter)
					default:
						log.Errorf("unknown NC sync name %s", ncSync.Name)
						return
					}
					if err != nil {
						log.Errorf("failed to run NC Get for %s: %v", ncSync.Name, err)
						continue
					}
					trans := netconf.NewXML2SchemapbConfigAdapter(t.schemaClient, t.schema)
					notif := trans.Transform(ctx, rsp.Doc)
					// debug
					rsp.Doc.WriteTo(os.Stdout)
					fmt.Println(prototext.Format(notif))
					// debug
					syncCh <- &SyncUpdate{
						Tree:   ncSync.Name,
						Update: notif,
					}
				}
			}
		}(ncSync)
	}
	wg.Wait()
}

func (t *ncTarget) Close() {
	t.driver.Close()
}
