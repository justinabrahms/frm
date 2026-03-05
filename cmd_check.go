package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/spf13/cobra"
)

type overdueContact struct {
	Name      string `json:"name"`
	Frequency string `json:"frequency"`
	LastSeen  string `json:"last_seen,omitempty"`
	Ago       string `json:"ago,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Org       string `json:"org,omitempty"`
	Group     string `json:"group,omitempty"`
	LastNote  string `json:"last_note,omitempty"`
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:     "check",
		Aliases: []string{"status"},
		Short:   "Show overdue contacts",
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

			jsonFlag, _ := cmd.Flags().GetBool("json")

			// For JSON output, build an index of last log entry per contact
			// (by path and name) so we can include last_note.
			lastEntry := make(map[string]*LogEntry)
			if jsonFlag {
				for i := range entries {
					e := &entries[i]
					if e.Path != "" {
						if prev, ok := lastEntry[e.Path]; !ok || e.Time.After(prev.Time) {
							lastEntry[e.Path] = e
						}
					}
					if prev, ok := lastEntry[e.Contact]; !ok || e.Time.After(prev.Time) {
						lastEntry[e.Contact] = e
					}
				}
			}

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

					isOverdue := !ok || now.Sub(last) > dur

					if !isOverdue {
						continue
					}

					oc := overdueContact{
						Name:      name,
						Frequency: freq,
					}
					if ok {
						oc.LastSeen = last.Format("2006-01-02")
						oc.Ago = formatAgo(now.Sub(last))
					}

					// Enrich with contact details for JSON consumers
					if jsonFlag {
						if email := obj.Card.PreferredValue(vcard.FieldEmail); email != "" {
							oc.Email = email
						}
						if phone := obj.Card.PreferredValue(vcard.FieldTelephone); phone != "" {
							oc.Phone = phone
						}
						if org := strings.TrimRight(obj.Card.PreferredValue(vcard.FieldOrganization), "; "); org != "" {
							oc.Org = org
						}
						if group := getGroup(obj.Card); group != "" {
							oc.Group = group
						}
						// Find last note from log entries
						le := lastEntry[obj.Path]
						if le == nil {
							le = lastEntry[name]
						}
						if le != nil && le.Note != "" {
							oc.LastNote = le.Note
						}
					}

					overdue = append(overdue, oc)
				}
			}

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
