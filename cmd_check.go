package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type overdueContact struct {
	Name      string `json:"name"`
	Frequency string `json:"frequency"`
	LastSeen  string `json:"last_seen,omitempty"`
	Ago       string `json:"ago,omitempty"`
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Show overdue contacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			results, err := allContactsMulti(cfg)
			if err != nil {
				return err
			}

			entries, err := readLog()
			if err != nil {
				return err
			}
			lastContact := lastContactTime(entries)

			now := time.Now()
			var overdue []overdueContact

			for _, r := range results {
				for _, obj := range r.objs {
					if isIgnored(obj.Card) || isSnoozed(obj.Card) {
						continue
					}
					freq := getFrequency(obj.Card)
					if freq == "" {
						continue
					}
					dur, err := parseDuration(freq)
					if err != nil {
						continue
					}
					name := contactName(obj)
					// Prefer path-based lookup (survives renames), fall back to name
					last, ok := lastContact[obj.Path]
					if !ok {
						last, ok = lastContact[name]
					}
					if !ok {
						overdue = append(overdue, overdueContact{
							Name:      name,
							Frequency: freq,
						})
						continue
					}
					if now.Sub(last) > dur {
						overdue = append(overdue, overdueContact{
							Name:      name,
							Frequency: freq,
							LastSeen:  last.Format("2006-01-02"),
							Ago:       formatAgo(now.Sub(last)),
						})
					}
				}
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				return printJSON(cmd, overdue)
			}

			if len(overdue) == 0 {
				fmt.Println("All caught up! No overdue contacts.")
			} else {
				fmt.Println("Overdue contacts:")
				var lines []string
				for _, o := range overdue {
					if o.Ago == "" {
						lines = append(lines, fmt.Sprintf("  %s (every %s, never contacted)", o.Name, o.Frequency))
					} else {
						lines = append(lines, fmt.Sprintf("  %s (every %s, last contact %s ago)", o.Name, o.Frequency, o.Ago))
					}
				}
				fmt.Println(strings.Join(lines, "\n"))
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
