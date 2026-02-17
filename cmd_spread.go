package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	spreadCmd := &cobra.Command{
		Use:   "spread",
		Short: "Spread never-contacted people across their frequency interval",
		Long: `After a big import, all tracked contacts show as overdue at once.
This command snoozes them with staggered dates so they come due evenly
over time, based on each contact's frequency.

For example, 10 monthly contacts get spaced ~3 days apart, so you
get a steady trickle instead of a wall of overdue contacts.

Only affects tracked contacts that have never been contacted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			apply, _ := cmd.Flags().GetBool("apply")

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

			type candidate struct {
				name   string
				freq   string
				dur    time.Duration
				rIndex int // index into results
				oIndex int // index into objs
			}
			freqGroups := make(map[string][]candidate)

			for ri, r := range results {
				for oi, obj := range r.objs {
					if isIgnored(obj.Card) || isSnoozed(obj.Card) {
						continue
					}
					freq := getFrequency(obj.Card)
					if freq == "" {
						continue
					}
					name := contactName(obj)
					if name == "" {
						continue
					}
					if _, ok := lastContact[obj.Path]; ok {
						continue
					}
					if _, ok := lastContact[name]; ok {
						continue
					}
					dur, err := parseDuration(freq)
					if err != nil {
						continue
					}
					freqGroups[freq] = append(freqGroups[freq], candidate{
						name:   name,
						freq:   freq,
						dur:    dur,
						rIndex: ri,
						oIndex: oi,
					})
				}
			}

			if len(freqGroups) == 0 {
				fmt.Println("No never-contacted tracked contacts to spread.")
				return nil
			}

			for freq := range freqGroups {
				sort.Slice(freqGroups[freq], func(i, j int) bool {
					return freqGroups[freq][i].name < freqGroups[freq][j].name
				})
			}

			now := time.Now()
			ctx := context.Background()
			var total int

			var freqs []string
			for freq := range freqGroups {
				freqs = append(freqs, freq)
			}
			sort.Strings(freqs)

			for _, freq := range freqs {
				group := freqGroups[freq]
				n := len(group)

				fmt.Printf("%s (%d contacts, every %s):\n", freq, n, freq)

				for _, c := range group {
					// Random date between now and the contact's frequency interval
					dueIn := time.Duration(rand.Int63n(int64(c.dur)))
					snoozeDate := now.Add(dueIn)
					dueInDays := int(dueIn.Hours() / 24)

					if !apply {
						fmt.Printf("  %s → due in %dd\n", c.name, dueInDays)
					} else {
						obj := &results[c.rIndex].objs[c.oIndex]
						client := results[c.rIndex].client
						setSnoozeUntil(obj.Card, snoozeDate)
						if _, err := client.PutAddressObject(ctx, obj.Path, obj.Card); err != nil {
							return fmt.Errorf("updating %s: %w", c.name, err)
						}
						fmt.Printf("  %s → due in %dd (snoozed until %s)\n", c.name, dueInDays, snoozeDate.Format("2006-01-02"))
					}
					total++
				}
			}

			if !apply {
				fmt.Printf("\nDry run: would snooze %d contacts. Run with --apply to execute.\n", total)
			} else {
				fmt.Printf("\nSpread %d contacts across their intervals.\n", total)
			}
			return nil
		},
	}
	spreadCmd.Flags().Bool("apply", false, "Actually apply the snoozes (default is dry run)")
	rootCmd.AddCommand(spreadCmd)
}
