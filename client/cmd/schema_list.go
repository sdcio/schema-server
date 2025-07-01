// Copyright 2024 Nokia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
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
			cfg := tablewriter.Config{
				Header: tw.CellConfig{
					Alignment:  tw.CellAlignment{Global: tw.AlignCenter},
					Formatting: tw.CellFormatting{AutoFormat: tw.On},
				},
				Row: tw.CellConfig{
					Alignment:  tw.CellAlignment{Global: tw.AlignLeft},
					Formatting: tw.CellFormatting{AutoWrap: tw.WrapTruncate},
				},
				MaxWidth: 80,
				Behavior: tw.Behavior{TrimSpace: tw.On},
			}
			table := tablewriter.NewTable(os.Stdout, tablewriter.WithConfig(cfg))
			table.Header([]string{"Name", "Vendor", "Version"})
			table.Append("Node1", "Ready")
			table.Bulk(tableData)
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
