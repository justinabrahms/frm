package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "ignore <name>",
		Short: "Ignore a contact so it never appears in triage or check",
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

			if isIgnored(obj.Card) {
				fmt.Printf("%s is already ignored\n", contactName(*obj))
				return nil
			}

			setIgnored(obj.Card)
			if _, err := client.PutAddressObject(context.Background(), obj.Path, obj.Card); err != nil {
				return fmt.Errorf("updating contact: %w", err)
			}
			fmt.Printf("Ignored %s\n", contactName(*obj))
			return nil
		},
	})
}
