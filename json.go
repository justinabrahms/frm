package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.PersistentFlags().Bool("json", false, "Output as JSON")
}

func isJSONMode(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func printJSON(cmd *cobra.Command, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

// printJSONError outputs a structured JSON error to stdout.
// Used when --json is set and a command fails, so callers parsing JSON
// get a machine-readable error instead of plain text on stderr.
func printJSONError(cmd *cobra.Command, err error) {
	data, _ := json.MarshalIndent(map[string]string{"error": err.Error()}, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
}
