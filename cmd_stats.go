package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "stats",
		Short: "Show contact tracking dashboard",
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

			var totalContacts, tracked, ignoredCount, overdueCount int
			for _, r := range results {
				for _, obj := range r.objs {
					if contactName(obj) == "" {
						continue
					}
					totalContacts++
					if isIgnored(obj.Card) {
						ignoredCount++
						continue
					}
					freq := getFrequency(obj.Card)
					if freq == "" {
						continue
					}
					tracked++
					dur, err := parseDuration(freq)
					if err != nil {
						continue
					}
					name := contactName(obj)
					last, ok := lastContact[obj.Path]
					if !ok {
						last, ok = lastContact[name]
					}
					if !ok || time.Now().Sub(last) > dur {
						overdueCount++
					}
				}
			}

			// Count interactions per contact
			contactCounts := make(map[string]int)
			for _, e := range entries {
				contactCounts[e.Contact]++
			}

			untriaged := totalContacts - tracked - ignoredCount
			var coveragePct float64
			if totalContacts > 0 {
				coveragePct = float64(tracked+ignoredCount) / float64(totalContacts) * 100
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				result := map[string]any{
					"total_contacts":     totalContacts,
					"tracked":            tracked,
					"ignored":            ignoredCount,
					"untriaged":          untriaged,
					"coverage_pct":       coveragePct,
					"overdue":            overdueCount,
					"total_interactions": len(entries),
				}
				if len(contactCounts) > 0 {
					most, least := mostLeastContacted(contactCounts)
					result["most_contacted"] = most
					result["least_contacted"] = least
				}
				return printJSON(cmd, result)
			}

			fmt.Printf("Contacts:        %d total\n", totalContacts)
			fmt.Printf("  Tracked:       %d\n", tracked)
			fmt.Printf("  Ignored:       %d\n", ignoredCount)
			fmt.Printf("  Untriaged:     %d (%.0f%%)\n", untriaged, 100-coveragePct)
			fmt.Printf("Overdue:         %d\n", overdueCount)
			fmt.Printf("Interactions:    %d\n", len(entries))

			if len(contactCounts) > 0 {
				most, least := mostLeastContacted(contactCounts)
				fmt.Printf("Most contacted:  %s (%d)\n", most, contactCounts[most])
				fmt.Printf("Least contacted: %s (%d)\n", least, contactCounts[least])
			}
			return nil
		},
	})
}

func mostLeastContacted(counts map[string]int) (most, least string) {
	type kv struct {
		name  string
		count int
	}
	var sorted []kv
	for name, count := range counts {
		sorted = append(sorted, kv{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return strings.ToLower(sorted[i].name) < strings.ToLower(sorted[j].name)
	})
	return sorted[0].name, sorted[len(sorted)-1].name
}
