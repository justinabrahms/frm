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
			obj, client, err := findContactMulti(cfg, args[0])
			if err != nil {
				return err
			}

			if !isIgnored(obj.Card) {
				fmt.Printf("%s is not ignored\n", contactName(*obj))
				return nil
			}

			removeIgnored(obj.Card)
			if _, err := client.PutAddressObject(context.Background(), obj.Path, obj.Card); err != nil {
				return fmt.Errorf("updating contact: %w", err)
			}
			fmt.Printf("Unignored %s\n", contactName(*obj))
			return nil
		},
	})
}
