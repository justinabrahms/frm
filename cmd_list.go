package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

type listEntry struct {
	Name      string `json:"name"`
	Frequency string `json:"frequency,omitempty"`
	Group     string `json:"group,omitempty"`
	DueIn     *int   `json:"due_in_days,omitempty"`
}

func init() {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List tracked contacts (use --all for everyone)",
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

			all, _ := cmd.Flags().GetBool("all")
			now := time.Now()

			var list []listEntry
			for _, r := range results {
				for _, obj := range r.objs {
					name := contactName(obj)
					if name == "" {
						continue
					}
					freq := getFrequency(obj.Card)
					if !all && (freq == "" || isIgnored(obj.Card)) {
						continue
					}

					e := listEntry{
						Name:      name,
						Frequency: freq,
						Group:     getGroup(obj.Card),
					}

					if freq != "" {
						dur, err := parseDuration(freq)
						if err == nil {
							// Check snooze first
							if snoozeUntil, ok := getSnoozeUntil(obj.Card); ok && now.Before(snoozeUntil) {
								days := int(snoozeUntil.Sub(now).Hours() / 24)
								e.DueIn = &days
							} else {
								last, ok := lastContact[obj.Path]
								if !ok {
									last, ok = lastContact[name]
								}
								if ok {
									days := int((dur - now.Sub(last)).Hours() / 24)
									e.DueIn = &days
								} else {
									days := 0
									e.DueIn = &days
								}
							}
						}
					}

					list = append(list, e)
				}
			}
			sort.Slice(list, func(i, j int) bool {
				return list[i].Name < list[j].Name
			})

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				return printJSON(cmd, list)
			}

			if len(list) == 0 {
				fmt.Println("No tracked contacts. Use 'frm track' to start tracking someone.")
				return nil
			}
			for _, e := range list {
				line := e.Name
				if e.Frequency != "" {
					line += fmt.Sprintf(" (every %s)", e.Frequency)
				}
				if e.Group != "" {
					line += fmt.Sprintf(" [%s]", e.Group)
				}
				if e.DueIn != nil {
					days := *e.DueIn
					if days < 0 {
						line += fmt.Sprintf(" — overdue by %dd", -days)
					} else if days == 0 {
						line += " — due now"
					} else {
						line += fmt.Sprintf(" — due in %dd", days)
					}
				}
				fmt.Println(line)
			}
			return nil
		},
	}
	listCmd.Flags().Bool("all", false, "List all contacts, not just tracked ones")
	rootCmd.AddCommand(listCmd)
}
