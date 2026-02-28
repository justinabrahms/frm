package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Update fields on an existing contact",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			obj, client, err := findContactMulti(cfg, name)
			if err != nil {
				return err
			}

			displayName := contactName(*obj)

			// Collect changes from explicitly provided flags.
			var changes []string

			email, _ := cmd.Flags().GetString("email")
			if cmd.Flags().Changed("email") {
				obj.Card[vcard.FieldEmail] = []*vcard.Field{{Value: email}}
				changes = append(changes, fmt.Sprintf("email=%s", email))
			}

			phone, _ := cmd.Flags().GetString("phone")
			if cmd.Flags().Changed("phone") {
				obj.Card[vcard.FieldTelephone] = []*vcard.Field{{Value: phone}}
				changes = append(changes, fmt.Sprintf("phone=%s", phone))
			}

			org, _ := cmd.Flags().GetString("org")
			if cmd.Flags().Changed("org") {
				obj.Card[vcard.FieldOrganization] = []*vcard.Field{{Value: org}}
				changes = append(changes, fmt.Sprintf("org=%s", org))
			}

			urls, _ := cmd.Flags().GetStringSlice("url")
			if cmd.Flags().Changed("url") {
				// Collect existing URLs for dedup.
				seen := make(map[string]bool)
				var merged []*vcard.Field
				for _, f := range obj.Card[vcard.FieldURL] {
					if f.Value != "" && !seen[f.Value] {
						seen[f.Value] = true
						merged = append(merged, f)
					}
				}
				for _, u := range urls {
					if u != "" && !seen[u] {
						seen[u] = true
						merged = append(merged, &vcard.Field{Value: u})
					}
				}
				obj.Card[vcard.FieldURL] = merged
				changes = append(changes, fmt.Sprintf("url=%s", strings.Join(urls, ",")))
			}

			if len(changes) == 0 {
				return fmt.Errorf("no fields to update (use --email, --phone, --org, or --url)")
			}

			dryRun := isDryRun(cmd)

			if !dryRun {
				ctx := context.Background()
				if _, err := client.PutAddressObject(ctx, obj.Path, obj.Card); err != nil {
					return fmt.Errorf("updating contact: %w", err)
				}
			}

			if isJSONMode(cmd) {
				out := map[string]interface{}{
					"action":  "edit",
					"name":    displayName,
					"changes": changes,
				}
				if dryRun {
					out["dry_run"] = true
				}
				return printJSON(cmd, out)
			}

			if dryRun {
				fmt.Printf("Would update %s: %s (dry run)\n", displayName, strings.Join(changes, ", "))
			} else {
				fmt.Printf("Updated %s: %s\n", displayName, strings.Join(changes, ", "))
			}
			return nil
		},
	}
	cmd.Flags().String("email", "", "email address")
	cmd.Flags().String("phone", "", "phone number")
	cmd.Flags().String("org", "", "organization")
	cmd.Flags().StringSlice("url", nil, "website or social URL (repeatable, deduped)")
	rootCmd.AddCommand(cmd)
}
