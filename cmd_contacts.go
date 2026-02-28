package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	contactsCmd := &cobra.Command{
		Use:    "contacts",
		Short:  "Deprecated: use 'frm list --all' instead",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(os.Stderr, "Warning: 'frm contacts' is deprecated, use 'frm list --all' instead.")

			// Find the list command and invoke it with --all
			listCmd, _, err := rootCmd.Find([]string{"list"})
			if err != nil {
				return fmt.Errorf("finding list command: %w", err)
			}

			// Set the --all flag on the list command
			if err := listCmd.Flags().Set("all", "true"); err != nil {
				return fmt.Errorf("setting --all flag: %w", err)
			}

			// Forward the --json flag if set
			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				if err := listCmd.Flags().Set("json", "true"); err != nil {
					return fmt.Errorf("setting --json flag: %w", err)
				}
			}

			return listCmd.RunE(listCmd, args)
		},
	}
	rootCmd.AddCommand(contactsCmd)
}
