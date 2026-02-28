package main

import (
	"bytes"
	"fmt"

	"github.com/emersion/go-vcard"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:    "dump <name>",
		Short:  "Dump raw vCard for a contact (debugging)",
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			obj, _, err := findContactMulti(cfg, args[0])
			if err != nil {
				return err
			}

			var buf bytes.Buffer
			enc := vcard.NewEncoder(&buf)
			if err := enc.Encode(obj.Card); err != nil {
				return fmt.Errorf("encoding vcard: %w", err)
			}
			fmt.Print(buf.String())
			return nil
		},
	}
	rootCmd.AddCommand(cmd)
}
