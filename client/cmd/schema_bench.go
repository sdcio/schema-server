/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"time"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"github.com/iptecharch/schema-server/utils"
)

// schemaBenchCmd represents the bench command
var schemaBenchCmd = &cobra.Command{
	Use:          "bench",
	Short:        "bench schemas",
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
		schemaKey := &sdcpb.Schema{
			Name:    schemaName,
			Vendor:  schemaVendor,
			Version: schemaVersion,
		}
		//
		req := &sdcpb.GetSchemaRequest{
			Path:   p,
			Schema: schemaKey,
			// WithDescription: withDesc,
		}
		start := time.Now()
		count := 0
		//
		ctx, cancel2 := context.WithTimeout(cmd.Context(), timeout)
		defer cancel2()
		rsp, err := schemaClient.GetSchema(ctx, req)
		if err != nil {
			return err
		}
		switch rsp := rsp.GetSchema().Schema.(type) {
		case *sdcpb.SchemaElem_Container:
			i, err := querySchemaContainer(ctx, schemaClient, rsp, req)
			if err != nil {
				return err
			}
			count += i
		case *sdcpb.SchemaElem_Field:
			count++
		case *sdcpb.SchemaElem_Leaflist:
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

func querySchemaContainer(ctx context.Context, schemaClient sdcpb.SchemaServerClient, cont *sdcpb.SchemaElem_Container, req *sdcpb.GetSchemaRequest) (int, error) {
	var err error
	count := 0
	for _, key := range cont.Container.GetKeys() {
		np := proto.Clone(req.GetPath()).(*sdcpb.Path)
		np.Elem = append(np.Elem, &sdcpb.PathElem{Name: key.GetName()})
		_, err = schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
			Path:   np,
			Schema: req.GetSchema(),
		})
		if err != nil {
			return 0, err
		}
		count++
	}
	for _, f := range cont.Container.GetFields() {
		np := proto.Clone(req.GetPath()).(*sdcpb.Path)
		np.Elem = append(np.Elem, &sdcpb.PathElem{Name: f.GetName()})
		_, err = schemaClient.GetSchema(ctx, &sdcpb.GetSchemaRequest{
			Path:   np,
			Schema: req.GetSchema(),
		})
		if err != nil {
			return 0, err
		}
		count++
	}

	for _, child := range cont.Container.GetChildren() {
		np := proto.Clone(req.GetPath()).(*sdcpb.Path)
		np.Elem = append(np.Elem, &sdcpb.PathElem{Name: child})
		req := &sdcpb.GetSchemaRequest{
			Path:   np,
			Schema: req.GetSchema(),
		}
		rsp, err := schemaClient.GetSchema(ctx, req)
		if err != nil {
			return 0, err
		}
		count++
		i, err := querySchemaContainer(ctx, schemaClient, rsp.GetSchema().Schema.(*sdcpb.SchemaElem_Container), req)
		if err != nil {
			return 0, err
		}
		count += i
	}

	return count, nil
}
