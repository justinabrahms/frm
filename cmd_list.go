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

			// Compute column widths
			nameW, freqW, groupW := 4, 4, 5 // "NAME", "FREQ", "GROUP"
			for _, e := range list {
				if len(e.Name) > nameW {
					nameW = len(e.Name)
				}
				if len(e.Frequency) > freqW {
					freqW = len(e.Frequency)
				}
				if len(e.Group) > groupW {
					groupW = len(e.Group)
				}
			}

			fmtStr := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%s\n", nameW, freqW, groupW)
			fmt.Printf(fmtStr, "NAME", "FREQ", "GROUP", "DUE")
			for _, e := range list {
				due := ""
				if e.DueIn != nil {
					days := *e.DueIn
					if days < 0 {
						due = fmt.Sprintf("overdue %dd", -days)
					} else if days == 0 {
						due = "now"
					} else {
						due = fmt.Sprintf("in %dd", days)
					}
				}
				fmt.Printf(fmtStr, e.Name, e.Frequency, e.Group, due)
			}
			return nil
		},
	}
	listCmd.Flags().Bool("all", false, "List all contacts, not just tracked ones")
	rootCmd.AddCommand(listCmd)
}
