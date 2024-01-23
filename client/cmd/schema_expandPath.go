/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/iptecharch/schema-server/pkg/utils"
	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

// schemaExpandPathCmd represents the expand-path command
var schemaExpandPathCmd = &cobra.Command{
	Use:          "expand-path",
	Aliases:      []string{"expand"},
	Short:        "given a path returns all sub-paths",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if configOnly && stateOnly {
			return errors.New("either --config-only or --state-only can be set")
		}
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
		dt := sdcpb.DataType_ALL
		if configOnly {
			dt = sdcpb.DataType_CONFIG
		}
		if stateOnly {
			dt = sdcpb.DataType_STATE
		}
		req := &sdcpb.ExpandPathRequest{
			Path:  p,
			Xpath: asXpath,
			Schema: &sdcpb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
			DataType: dt,
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		ctx, cancel2 := context.WithTimeout(cmd.Context(), timeout)
		defer cancel2()
		rsp, err := schemaClient.ExpandPath(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		fmt.Fprintf(os.Stderr, "path count: %d | %d\n", len(rsp.GetPath()), len(rsp.GetXpath()))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaExpandPathCmd)
	schemaExpandPathCmd.Flags().StringVarP(&xpath, "path", "", "", "xpath to expand")
	schemaExpandPathCmd.Flags().BoolVarP(&asXpath, "xpath", "", false, "return paths in xpath format")
	schemaExpandPathCmd.Flags().BoolVarP(&configOnly, "config-only", "", false, "return paths from the config tree only")
	schemaExpandPathCmd.Flags().BoolVarP(&stateOnly, "state-only", "", false, "return paths from the config tree only")
}

var asXpath bool
var configOnly bool
var stateOnly bool
