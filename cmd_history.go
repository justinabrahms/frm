package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "history <name>",
		Short: "Show interaction log for a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := readLog()
			if err != nil {
				return err
			}

			name := args[0]
			nameLower := strings.ToLower(name)
			var found []LogEntry
			for _, e := range entries {
				if strings.ToLower(e.Contact) == nameLower {
					found = append(found, e)
				}
			}

			if len(found) == 0 {
				fmt.Printf("No interactions logged for %s\n", name)
				return nil
			}

			json, _ := cmd.Flags().GetBool("json")
			if json {
				return printJSON(cmd, found)
			}

			for _, e := range found {
				line := e.Time.Format("2006-01-02")
				if e.Note != "" {
					line += "  " + e.Note
				}
				fmt.Println(line)
			}
			return nil
		},
	})
}
