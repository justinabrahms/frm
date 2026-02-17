package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	logCmd := &cobra.Command{
		Use:   "log <name>",
		Short: "Log an interaction with a contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			note, _ := cmd.Flags().GetString("note")
			when, _ := cmd.Flags().GetString("when")

			var ts time.Time
			if when != "" {
				var err error
				ts, err = parseWhen(when)
				if err != nil {
					return err
				}
			} else {
				ts = time.Now().UTC()
			}

			entry := LogEntry{
				Contact: args[0],
				Time:    ts,
				Note:    note,
			}

			// Try to resolve the contact path for name normalization
			cfg, cfgErr := loadConfig()
			if cfgErr == nil {
				obj, _, err := findContactMulti(cfg, args[0])
				if err == nil {
					entry.Path = obj.Path
					entry.Contact = contactName(*obj)
				}
			}

			if err := appendLog(entry); err != nil {
				return err
			}
			fmt.Printf("Logged interaction with %s\n", entry.Contact)
			return nil
		},
	}
	logCmd.Flags().String("note", "", "Note about the interaction")
	logCmd.Flags().String("when", "", "When it happened (e.g. 2024-01-15 or -2w)")
	rootCmd.AddCommand(logCmd)
}
