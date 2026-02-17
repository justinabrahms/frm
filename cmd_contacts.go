package main

import (
	"context"
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
			client, err := newCardDAVClient(cfg)
			if err != nil {
				return err
			}
			ctx := context.Background()
			book, err := findAddressBook(ctx, client)
			if err != nil {
				return err
			}
			objs, err := queryAllContacts(ctx, client, book)
			if err != nil {
				return err
			}

			var names []string
			for _, obj := range objs {
				name := contactName(obj)
				if name != "" {
					names = append(names, name)
				}
			}
			sort.Strings(names)
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
