package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	setCmd := &cobra.Command{
		Use:   "set <name> <group>",
		Short: "Assign a contact to a group (e.g. close-friends, professional, family)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			matches, err := findAllContactsMulti(cfg, args[0])
			if err != nil {
				return err
			}

			ctx := context.Background()
			for _, m := range matches {
				setGroup(m.obj.Card, args[1])
				if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
			}

			name := contactName(*matches[0].obj)
			if len(matches) > 1 {
				fmt.Printf("Set %s group to %s (%d accounts)\n", name, args[1], len(matches))
			} else {
				fmt.Printf("Set %s group to %s\n", name, args[1])
			}
			return nil
		},
	}

	unsetCmd := &cobra.Command{
		Use:   "unset <name>",
		Short: "Remove a contact from its group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			matches, err := findAllContactsMulti(cfg, args[0])
			if err != nil {
				return err
			}

			ctx := context.Background()
			for _, m := range matches {
				removeGroup(m.obj.Card)
				if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
			}

			name := contactName(*matches[0].obj)
			if len(matches) > 1 {
				fmt.Printf("Removed group from %s (%d accounts)\n", name, len(matches))
			} else {
				fmt.Printf("Removed group from %s\n", name)
			}
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list [group]",
		Short: "List groups or contacts in a group",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			results, err := allContactsMulti(cfg)
			if err != nil {
				return err
			}

			if len(args) == 1 {
				// List contacts in a specific group
				groupName := strings.ToLower(args[0])
				var names []string
				for _, r := range results {
					for _, obj := range r.objs {
						if strings.ToLower(getGroup(obj.Card)) == groupName {
							names = append(names, contactName(obj))
						}
					}
				}
				sort.Strings(names)

				jsonFlag, _ := cmd.Flags().GetBool("json")
				if jsonFlag {
					return printJSON(cmd, names)
				}
				for _, name := range names {
					fmt.Println(name)
				}
				if len(names) == 0 {
					fmt.Printf("No contacts in group %q\n", args[0])
				}
			} else {
				// List all groups with counts
				groups := make(map[string]int)
				for _, r := range results {
					for _, obj := range r.objs {
						g := getGroup(obj.Card)
						if g != "" {
							groups[g]++
						}
					}
				}
				type kv struct {
					name  string
					count int
				}
				var sorted []kv
				for name, count := range groups {
					sorted = append(sorted, kv{name, count})
				}
				sort.Slice(sorted, func(i, j int) bool {
					return sorted[i].name < sorted[j].name
				})

				jsonFlag, _ := cmd.Flags().GetBool("json")
				if jsonFlag {
					return printJSON(cmd, groups)
				}
				for _, g := range sorted {
					fmt.Printf("  %s (%d)\n", g.name, g.count)
				}
				if len(sorted) == 0 {
					fmt.Println("No groups defined")
				}
			}
			return nil
		},
	}

	groupCmd := &cobra.Command{
		Use:   "group",
		Short: "Manage contact groups",
	}
	groupCmd.AddCommand(setCmd, unsetCmd, listCmd)
	rootCmd.AddCommand(groupCmd)
}
