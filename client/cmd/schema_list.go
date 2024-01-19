/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"

	sdcpb "github.com/iptecharch/sdc-protos/sdcpb"
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
		req := &sdcpb.ListSchemaRequest{}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))
		ctx, cancel2 := context.WithTimeout(cmd.Context(), timeout)
		defer cancel2()
		schemaList, err := schemaClient.ListSchema(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		switch format {
		case "table", "":
			tableData := make([][]string, 0, len(schemaList.GetSchema()))
			for _, schema := range schemaList.GetSchema() {
				tableData = append(tableData, []string{schema.GetName(), schema.GetVendor(), schema.GetVersion()})
			}
			sort.Slice(tableData, func(i, j int) bool {
				if tableData[i][0] == tableData[j][0] {
					if tableData[i][1] == tableData[j][1] {
						return tableData[i][2] < tableData[j][2]
					}
					return tableData[i][1] < tableData[j][1]
				}
				return tableData[i][0] < tableData[j][0]
			})
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Name", "Vendor", "Version"})
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetAutoFormatHeaders(false)
			table.SetAutoWrapText(false)
			table.AppendBulk(tableData)
			table.Render()
		case "proto":
			fmt.Println(prototext.Format(schemaList))
		case "json":
			b, err := json.MarshalIndent(schemaList.GetSchema(), "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
		}

		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaListCmd)
}
