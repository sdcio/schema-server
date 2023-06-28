/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"os"
	"time"

	schemapb "github.com/iptecharch/schema-server/protos/schema_server"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use: "schema-client",
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

func init() {
	rootCmd.PersistentFlags().StringVarP(&addr, "address", "a", "localhost:55000", "schema server address")
	rootCmd.PersistentFlags().StringVar(&schemaName, "name", "", "schema name")
	rootCmd.PersistentFlags().StringVar(&schemaVendor, "vendor", "", "schema vendor")
	rootCmd.PersistentFlags().StringVar(&schemaVersion, "version", "", "schema version")
	rootCmd.PersistentFlags().StringVar(&format, "format", "", "output format")
}

func createSchemaClient(ctx context.Context, addr string) (schemapb.SchemaServerClient, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cc, err := grpc.DialContext(ctx, addr,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
	)
	if err != nil {
		return nil, err
	}
	return schemapb.NewSchemaServerClient(cc), nil
}
