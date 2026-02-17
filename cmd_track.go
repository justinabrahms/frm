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
			client, err := newCardDAVClient(cfg)
			if err != nil {
				return err
			}
			ctx := context.Background()
			book, err := findAddressBook(ctx, client)
			if err != nil {
				return err
			}
			obj, err := findContactByName(ctx, client, book, args[0])
			if err != nil {
				return err
			}

			setFrequency(obj.Card, every)
			if _, err := client.PutAddressObject(ctx, obj.Path, obj.Card); err != nil {
				return fmt.Errorf("updating contact: %w", err)
			}
			fmt.Printf("Tracking %s every %s\n", contactName(*obj), every)
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
			client, err := newCardDAVClient(cfg)
			if err != nil {
				return err
			}
			ctx := context.Background()
			book, err := findAddressBook(ctx, client)
			if err != nil {
				return err
			}
			obj, err := findContactByName(ctx, client, book, args[0])
			if err != nil {
				return err
			}

			removeFrequency(obj.Card)
			if _, err := client.PutAddressObject(ctx, obj.Path, obj.Card); err != nil {
				return fmt.Errorf("updating contact: %w", err)
			}
			fmt.Printf("Stopped tracking %s\n", contactName(*obj))
			return nil
		},
	}

	rootCmd.AddCommand(trackCmd)
	rootCmd.AddCommand(untrackCmd)
}
