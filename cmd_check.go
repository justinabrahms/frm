package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Show overdue contacts",
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
			objs, err := queryAllContacts(ctx, client, book)
			if err != nil {
				return err
			}

			entries, err := readLog()
			if err != nil {
				return err
			}
			lastContact := lastContactTime(entries)

			now := time.Now()
			var overdue []string

			for _, obj := range objs {
				freq := getFrequency(obj.Card)
				if freq == "" {
					continue
				}
				dur, err := parseDuration(freq)
				if err != nil {
					continue
				}
				name := contactName(obj)
				last, ok := lastContact[name]
				if !ok {
					overdue = append(overdue, fmt.Sprintf("  %s (every %s, never contacted)", name, freq))
					continue
				}
				if now.Sub(last) > dur {
					ago := formatAgo(now.Sub(last))
					overdue = append(overdue, fmt.Sprintf("  %s (every %s, last contact %s ago)", name, freq, ago))
				}
			}

			if len(overdue) == 0 {
				fmt.Println("All caught up! No overdue contacts.")
			} else {
				fmt.Println("Overdue contacts:")
				fmt.Println(strings.Join(overdue, "\n"))
			}
			return nil
		},
	})
}

func formatAgo(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}
	if days < 30 {
		return fmt.Sprintf("%dw", days/7)
	}
	return fmt.Sprintf("%dm", days/30)
}
