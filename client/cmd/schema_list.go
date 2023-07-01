/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
)

// schemaListCmd represents the list command
var schemaListCmd = &cobra.Command{
	Use:          "list",
	Short:        "list schemas",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &schemapb.ListSchemaRequest{}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		schemaList, err := schemaClient.ListSchema(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		switch format {
		case "table":
			tableData := make([][]string, 0, len(schemaList.GetSchema()))
			for _, schema := range schemaList.GetSchema() {
				tableData = append(tableData, []string{schema.GetName(), schema.GetVendor(), schema.GetVersion()})
			}
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Name", "Vendor", "Version"})
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetAutoFormatHeaders(false)
			table.SetAutoWrapText(false)
			table.AppendBulk(tableData)
			table.Render()
		default:
			fmt.Println(prototext.Format(schemaList))
		}

		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaListCmd)
}
