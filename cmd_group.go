package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// groupMembersRunE lists contacts belonging to a specific group.
func groupMembersRunE(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	results, err := allContactsMulti(cfg)
	if err != nil {
		return err
	}

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
	return nil
}

// groupListRunE lists all groups with their contact counts.
func groupListRunE(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	results, err := allContactsMulti(cfg)
	if err != nil {
		return err
	}

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
	return nil
}

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

			name := contactName(*matches[0].obj)
			dryRun := isDryRun(cmd)

			if !dryRun {
				ctx := context.Background()
				for _, m := range matches {
					setGroup(m.obj.Card, args[1])
					if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
						return fmt.Errorf("updating contact: %w", err)
					}
				}
			}

			if isJSONMode(cmd) {
				out := map[string]interface{}{
					"action":   "group_set",
					"name":     name,
					"group":    args[1],
					"accounts": len(matches),
				}
				if dryRun {
					out["dry_run"] = true
				}
				return printJSON(cmd, out)
			}

			if dryRun {
				fmt.Printf("Would set %s group to %s (dry run)\n", name, args[1])
			} else if len(matches) > 1 {
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

			name := contactName(*matches[0].obj)
			dryRun := isDryRun(cmd)

			if !dryRun {
				ctx := context.Background()
				for _, m := range matches {
					removeGroup(m.obj.Card)
					if _, err := m.client.PutAddressObject(ctx, m.obj.Path, m.obj.Card); err != nil {
						return fmt.Errorf("updating contact: %w", err)
					}
				}
			}

			if isJSONMode(cmd) {
				out := map[string]interface{}{
					"action":   "group_unset",
					"name":     name,
					"accounts": len(matches),
				}
				if dryRun {
					out["dry_run"] = true
				}
				return printJSON(cmd, out)
			}

			if dryRun {
				fmt.Printf("Would remove group from %s (dry run)\n", name)
			} else if len(matches) > 1 {
				fmt.Printf("Removed group from %s (%d accounts)\n", name, len(matches))
			} else {
				fmt.Printf("Removed group from %s\n", name)
			}
			return nil
		},
	}

	membersCmd := &cobra.Command{
		Use:   "members <group>",
		Short: "List contacts in a specific group",
		Args:  cobra.ExactArgs(1),
		RunE:  groupMembersRunE,
	}

	listCmd := &cobra.Command{
		Use:   "list [group]",
		Short: "List all groups",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				// Backwards compatibility: "group list <group>" delegates to "group members"
				fmt.Fprintf(os.Stderr, "Warning: 'frm group list <group>' is deprecated, use 'frm group members <group>' instead\n")
				return groupMembersRunE(cmd, args)
			}
			return groupListRunE(cmd, args)
		},
	}

	groupCmd := &cobra.Command{
		Use:   "group",
		Short: "Manage contact groups",
	}
	groupCmd.AddCommand(setCmd, unsetCmd, listCmd, membersCmd)
	rootCmd.AddCommand(groupCmd)
}
