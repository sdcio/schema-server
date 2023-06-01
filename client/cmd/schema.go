/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

var schemaName string
var schemaVendor string
var schemaVersion string

// schemaCmd represents the schema command
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "query/change schema(s)",
}

func init() {
	rootCmd.AddCommand(schemaCmd)
}
