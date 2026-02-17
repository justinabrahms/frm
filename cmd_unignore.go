package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "unignore <name>",
		Short: "Remove ignore flag from a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			matches, err := findAllContactsMulti(cfg, args[0])
			if err != nil {
				return err
			}

			ctx := context.Background()
			var updated int
			for _, m := range matches {
				if !isIgnored(m.obj.Card) {
					continue
				}
				removeIgnored(m.obj.Card)
				if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
				updated++
			}

			name := contactName(*matches[0].obj)
			if updated == 0 {
				fmt.Printf("%s is not ignored\n", name)
			} else if updated > 1 {
				fmt.Printf("Unignored %s (%d accounts)\n", name, updated)
			} else {
				fmt.Printf("Unignored %s\n", name)
			}
			return nil
		},
	})
}
