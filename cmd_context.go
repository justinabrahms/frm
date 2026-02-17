package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:     "context <name>",
		Aliases: []string{"show", "detail"},
		Short:   "Pre-meeting prep: show contact summary",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			obj, _, err := findContactMulti(cfg, args[0])
			if err != nil {
				return err
			}

			name := contactName(*obj)
			freq := getFrequency(obj.Card)
			ignored := isIgnored(obj.Card)
			group := getGroup(obj.Card)

			entries, err := readLog()
			if err != nil {
				return err
			}

			nameLower := strings.ToLower(name)
			var lastEntry *LogEntry
			for i := len(entries) - 1; i >= 0; i-- {
				if strings.ToLower(entries[i].Contact) == nameLower || entries[i].Path == obj.Path {
					lastEntry = &entries[i]
					break
				}
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				result := map[string]any{
					"name":    name,
					"ignored": ignored,
				}
				if freq != "" {
					result["frequency"] = freq
				}
				if group != "" {
					result["group"] = group
				}
				if lastEntry != nil {
					result["last_contact"] = lastEntry.Time.Format(time.RFC3339)
					if lastEntry.Note != "" {
						result["last_note"] = lastEntry.Note
					}
					result["days_since"] = int(time.Since(lastEntry.Time).Hours() / 24)
				}
				if freq != "" {
					dur, err := parseDuration(freq)
					if err == nil {
						if lastEntry != nil {
							daysUntil := int((dur - time.Since(lastEntry.Time)).Hours() / 24)
							result["days_until_due"] = daysUntil
						} else {
							result["days_until_due"] = 0
						}
					}
				}
				providers := initProviders(cfg)
				if lines := collectContext(providers, obj.Card); len(lines) > 0 {
					result["providers"] = lines
				}
				return printJSON(cmd, result)
			}

			fmt.Printf("Name:      %s\n", name)
			if ignored {
				fmt.Println("Status:    ignored")
			}
			if group != "" {
				fmt.Printf("Group:     %s\n", group)
			}
			if freq != "" {
				fmt.Printf("Frequency: every %s\n", freq)
			} else {
				fmt.Println("Frequency: not tracked")
			}

			if lastEntry != nil {
				daysSince := int(time.Since(lastEntry.Time).Hours() / 24)
				fmt.Printf("Last seen: %s (%d days ago)\n", lastEntry.Time.Format("2006-01-02"), daysSince)
				if lastEntry.Note != "" {
					fmt.Printf("Last note: %s\n", lastEntry.Note)
				}
				if freq != "" {
					dur, err := parseDuration(freq)
					if err == nil {
						daysUntil := int((dur - time.Since(lastEntry.Time)).Hours() / 24)
						if daysUntil < 0 {
							fmt.Printf("Status:    overdue by %d days\n", -daysUntil)
						} else {
							fmt.Printf("Due in:    %d days\n", daysUntil)
						}
					}
				}
			} else {
				fmt.Println("Last seen: never")
				if freq != "" {
					fmt.Println("Status:    overdue (never contacted)")
				}
			}

			providers := initProviders(cfg)
			if lines := collectContext(providers, obj.Card); len(lines) > 0 {
				fmt.Println()
				for _, line := range lines {
					fmt.Println(line)
				}
			}
			return nil
		},
	})
}
