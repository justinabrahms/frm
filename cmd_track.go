package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	trackCmd := &cobra.Command{
		Use:   "track <name>",
		Short: "Set contact frequency (e.g. --every 2w)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			every, _ := cmd.Flags().GetString("every")
			if every == "" {
				return fmt.Errorf("--every flag is required (e.g. 2w, 1m, 3d)")
			}
			if _, err := parseDuration(every); err != nil {
				return err
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			matches, err := findAllContactsMulti(cfg, args[0])
			if err != nil {
				return err
			}

			ctx := context.Background()
			for _, m := range matches {
				setFrequency(m.obj.Card, every)
				if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
			}

			name := contactName(*matches[0].obj)
			if len(matches) > 1 {
				fmt.Printf("Tracking %s every %s (%d accounts)\n", name, every, len(matches))
			} else {
				fmt.Printf("Tracking %s every %s\n", name, every)
			}
			return nil
		},
	}
	trackCmd.Flags().String("every", "", "Contact frequency (e.g. 2w, 1m, 3d)")

	untrackCmd := &cobra.Command{
		Use:   "untrack <name>",
		Short: "Stop tracking contact frequency",
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
			for _, m := range matches {
				removeFrequency(m.obj.Card)
				if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
			}

			name := contactName(*matches[0].obj)
			if len(matches) > 1 {
				fmt.Printf("Stopped tracking %s (%d accounts)\n", name, len(matches))
			} else {
				fmt.Printf("Stopped tracking %s\n", name)
			}
			return nil
		},
	}

	rootCmd.AddCommand(trackCmd)
	rootCmd.AddCommand(untrackCmd)
}
