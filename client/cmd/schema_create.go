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
	"fmt"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/prototext"
)

var schemaFiles []string
var schemaDirs []string
var schemaExcludes []string

// schemaCreateCmd represents the create command
var schemaCreateCmd = &cobra.Command{
	Use:          "create",
	Short:        "create a schema",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()
		schemaClient, err := createSchemaClient(ctx, addr)
		if err != nil {
			return err
		}
		req := &sdcpb.CreateSchemaRequest{
			Schema: &sdcpb.Schema{
				Name:    schemaName,
				Vendor:  schemaVendor,
				Version: schemaVersion,
			},
			File:      schemaFiles,
			Directory: schemaDirs,
			Exclude:   schemaExcludes,
		}
		fmt.Println("request:")
		fmt.Println(prototext.Format(req))

		ctx, cancel2 := context.WithTimeout(cmd.Context(), timeout)
		defer cancel2()
		rsp, err := schemaClient.CreateSchema(ctx, req)
		if err != nil {
			return err
		}
		fmt.Println("response:")
		fmt.Println(prototext.Format(rsp))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaCreateCmd)
	schemaCreateCmd.Flags().StringArrayVarP(&schemaFiles, "file", "", []string{}, "path to file containing a YANG module")
	schemaCreateCmd.Flags().StringArrayVarP(&schemaDirs, "dir", "", []string{}, "path to file containing a YANG module dependency")
	schemaCreateCmd.Flags().StringArrayVarP(&schemaExcludes, "exclude", "", []string{}, "regex of modules names to be excluded")
}
