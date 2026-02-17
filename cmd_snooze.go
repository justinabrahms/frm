package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	snoozeCmd := &cobra.Command{
		Use:   "snooze <name>",
		Short: "Snooze a contact until a future date",
		Long:  "Suppress a contact from check/triage until a given date. Use --until with an absolute date (2026-04-01) or relative duration (2m, 6w).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			until, _ := cmd.Flags().GetString("until")
			if until == "" {
				return fmt.Errorf("--until is required")
			}
			t, err := parseUntil(until)
			if err != nil {
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
				setSnoozeUntil(m.obj.Card, t)
				if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
			}

			name := contactName(*matches[0].obj)
			if len(matches) > 1 {
				fmt.Printf("Snoozed %s until %s (%d accounts)\n", name, t.Format("2006-01-02"), len(matches))
			} else {
				fmt.Printf("Snoozed %s until %s\n", name, t.Format("2006-01-02"))
			}
			return nil
		},
	}
	snoozeCmd.Flags().String("until", "", "Date to snooze until (e.g. 2026-04-01 or 2m)")
	rootCmd.AddCommand(snoozeCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "unsnooze <name>",
		Short: "Remove snooze from a contact",
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
				if _, ok := getSnoozeUntil(m.obj.Card); !ok {
					continue
				}
				removeSnoozeUntil(m.obj.Card)
				if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
				updated++
			}

			name := contactName(*matches[0].obj)
			if updated == 0 {
				fmt.Printf("%s is not snoozed\n", name)
			} else if updated > 1 {
				fmt.Printf("Unsnoozed %s (%d accounts)\n", name, updated)
			} else {
				fmt.Printf("Unsnoozed %s\n", name)
			}
			return nil
		},
	})
}
