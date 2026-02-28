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

			dryRun := isDryRun(cmd)

			if !dryRun {
				if err := appendLog(entry); err != nil {
					return err
				}
			}

			if isJSONMode(cmd) {
				out := map[string]interface{}{
					"action":  "log",
					"contact": entry.Contact,
					"time":    entry.Time.Format(time.RFC3339),
					"note":    entry.Note,
				}
				if dryRun {
					out["dry_run"] = true
				}
				return printJSON(cmd, out)
			}

			if dryRun {
				fmt.Printf("Would log interaction with %s (dry run)\n", entry.Contact)
			} else {
				fmt.Printf("Logged interaction with %s\n", entry.Contact)
			}
			return nil
		},
	}
	logCmd.Flags().String("note", "", "Note about the interaction")
	logCmd.Flags().String("when", "", "When it happened (YYYY-MM-DD, e.g. 2024-01-15)")
	rootCmd.AddCommand(logCmd)
}
