package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "contacts",
		Short: "List contacts from CardDAV",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			results, err := allContactsMulti(cfg)
			if err != nil {
				return err
			}

			var names []string
			for _, r := range results {
				for _, obj := range r.objs {
					name := contactName(obj)
					if name != "" {
						names = append(names, name)
					}
				}
			}
			sort.Strings(names)

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				return printJSON(cmd, names)
			}

			for _, name := range names {
				fmt.Println(name)
			}
			if len(names) == 0 {
				fmt.Fprintln(os.Stderr, "No contacts found.")
			}
			return nil
		},
	})
}
