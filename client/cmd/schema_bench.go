/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/iptecharch/schema-server/utils"
)

// schemaBenchCmd represents the bench command
var schemaBenchCmd = &cobra.Command{
	Use:           "bench",
	Short:         "bench schemas",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := utils.ParsePath(xpath)
		if err != nil {
			return err
		}

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		schemaKey := &schemapb.Schema{
			Name:    schemaName,
			Vendor:  schemaVendor,
			Version: schemaVersion,
		}
		//
		req := &schemapb.GetSchemaRequest{
			Path:   p,
			Schema: schemaKey,
			// WithDescription: withDesc,
		}
		start := time.Now()
		count := 0
		//
		rsp, err := schemaClient.GetSchema(ctx, req)
		if err != nil {
			return err
		}
		switch rsp := rsp.GetSchema().Schema.(type) {
		case *schemapb.SchemaElem_Container:
			i, err := querySchemaContainer(ctx, schemaClient, rsp, req)
			if err != nil {
				return err
			}
			count += i
		case *schemapb.SchemaElem_Field:
			count++
		case *schemapb.SchemaElem_Leaflist:
			count++
		}

		fmt.Printf("queried %d object(s) in %s\n", count, time.Since(start))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaBenchCmd)
	schemaBenchCmd.Flags().StringVarP(&xpath, "path", "", "/", "xpath to start recursive schema gets from")
}

func querySchemaContainer(ctx context.Context, schemaClient schemapb.SchemaServerClient, cont *schemapb.SchemaElem_Container, req *schemapb.GetSchemaRequest) (int, error) {
	var err error
	count := 0
	for _, key := range cont.Container.GetKeys() {
		np := proto.Clone(req.GetPath()).(*schemapb.Path)
		np.Elem = append(np.Elem, &schemapb.PathElem{Name: key.GetName()})
		_, err = schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
			Path:   np,
			Schema: req.GetSchema(),
		})
		if err != nil {
			return 0, err
		}
		count++
	}
	for _, f := range cont.Container.GetFields() {
		np := proto.Clone(req.GetPath()).(*schemapb.Path)
		np.Elem = append(np.Elem, &schemapb.PathElem{Name: f.GetName()})
		_, err = schemaClient.GetSchema(ctx, &schemapb.GetSchemaRequest{
			Path:   np,
			Schema: req.GetSchema(),
		})
		if err != nil {
			return 0, err
		}
		count++
	}

	for _, child := range cont.Container.GetChildren() {
		np := proto.Clone(req.GetPath()).(*schemapb.Path)
		np.Elem = append(np.Elem, &schemapb.PathElem{Name: child})
		req := &schemapb.GetSchemaRequest{
			Path:   np,
			Schema: req.GetSchema(),
		}
		rsp, err := schemaClient.GetSchema(ctx, req)
		if err != nil {
			return 0, err
		}
		count++
		i, err := querySchemaContainer(ctx, schemaClient, rsp.GetSchema().Schema.(*schemapb.SchemaElem_Container), req)
		if err != nil {
			return 0, err
		}
		count += i
	}

	return count, nil
}
