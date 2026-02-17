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
			obj, client, err := findContactMulti(cfg, args[0])
			if err != nil {
				return err
			}

			setSnoozeUntil(obj.Card, t)
			if _, err := client.PutAddressObject(context.Background(), obj.Path, obj.Card); err != nil {
				return fmt.Errorf("updating contact: %w", err)
			}
			fmt.Printf("Snoozed %s until %s\n", contactName(*obj), t.Format("2006-01-02"))
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
			obj, client, err := findContactMulti(cfg, args[0])
			if err != nil {
				return err
			}

			if _, ok := getSnoozeUntil(obj.Card); !ok {
				fmt.Printf("%s is not snoozed\n", contactName(*obj))
				return nil
			}

			removeSnoozeUntil(obj.Card)
			if _, err := client.PutAddressObject(context.Background(), obj.Path, obj.Card); err != nil {
				return fmt.Errorf("updating contact: %w", err)
			}
			fmt.Printf("Unsnoozed %s\n", contactName(*obj))
			return nil
		},
	})
}
