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
	"os"
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "schemac",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

var addr string
var format string
var maxRcvMsg int
var timeout time.Duration

func init() {
	rootCmd.PersistentFlags().StringVarP(&addr, "address", "a", "localhost:55000", "schema server address")
	rootCmd.PersistentFlags().StringVar(&schemaName, "name", "", "schema name")
	rootCmd.PersistentFlags().StringVar(&schemaVendor, "vendor", "", "schema vendor")
	rootCmd.PersistentFlags().StringVar(&schemaVersion, "version", "", "schema version")
	rootCmd.PersistentFlags().StringVar(&format, "format", "", "output format")
	rootCmd.PersistentFlags().IntVar(&maxRcvMsg, "max-rcv-msg", 25165824, "the maximum message size in bytes the client can receive")
	rootCmd.PersistentFlags().DurationVarP(&timeout, "timeout", "", 60*time.Second, "gRPC rpc timeout")
}

func createSchemaClient(ctx context.Context, addr string) (sdcpb.SchemaServerClient, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cc, err := grpc.DialContext(ctx, addr,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxRcvMsg)),
	)
	if err != nil {
		return nil, err
	}
	return sdcpb.NewSchemaServerClient(cc), nil
}
