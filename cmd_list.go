package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
)

func init() {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tracked contacts (use --all for everyone)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			results, err := allContactsMulti(cfg)
			if err != nil {
				return err
			}

			all, _ := cmd.Flags().GetBool("all")

			var names []string
			for _, r := range results {
				for _, obj := range r.objs {
					name := contactName(obj)
					if name == "" {
						continue
					}
					if all {
						names = append(names, name)
						continue
					}
					if getFrequency(obj.Card) != "" && !isIgnored(obj.Card) {
						names = append(names, name)
					}
				}
			}
			sort.Strings(names)

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				return printJSON(cmd, names)
			}

			if len(names) == 0 {
				fmt.Println("No tracked contacts. Use 'frm track' to start tracking someone.")
				return nil
			}
			for _, name := range names {
				fmt.Println(name)
			}
			return nil
		},
	}
	listCmd.Flags().Bool("all", false, "List all contacts, not just tracked ones")
	rootCmd.AddCommand(listCmd)
}
