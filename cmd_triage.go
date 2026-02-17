package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav/carddav"
	"github.com/spf13/cobra"
)

type triageContact struct {
	obj    carddav.AddressObject
	client *carddav.Client
}

func init() {
	triageCmd := &cobra.Command{
		Use:   "triage",
		Short: "Walk through untagged contacts and assign frequencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			results, err := allContactsMulti(cfg)
			if err != nil {
				return err
			}

			// Filter to untriaged contacts (no frequency, not ignored)
			var untriaged []triageContact
			for _, r := range results {
				for _, obj := range r.objs {
					if getFrequency(obj.Card) == "" && !isIgnored(obj.Card) {
						if contactName(obj) != "" {
							untriaged = append(untriaged, triageContact{obj: obj, client: r.client})
						}
					}
				}
			}

			// Sort alphabetically
			sort.Slice(untriaged, func(i, j int) bool {
				return contactName(untriaged[i].obj) < contactName(untriaged[j].obj)
			})

			// Apply limit
			limit, _ := cmd.Flags().GetInt("limit")
			if limit >= 0 && limit < len(untriaged) {
				untriaged = untriaged[:limit]
			}

			jsonFlag, _ := cmd.Flags().GetBool("json")
			if jsonFlag {
				providers := initProviders(cfg)
				out := make([]map[string]any, 0, len(untriaged))
				for _, tc := range untriaged {
					entry := map[string]any{
						"name": contactName(tc.obj),
					}
					if email := tc.obj.Card.PreferredValue(vcard.FieldEmail); email != "" {
						entry["email"] = email
					}
					if org := strings.TrimRight(tc.obj.Card.PreferredValue(vcard.FieldOrganization), "; "); org != "" {
						entry["org"] = org
					}
					if tel := tc.obj.Card.PreferredValue(vcard.FieldTelephone); tel != "" {
						entry["phone"] = tel
					}
					if lines := collectContext(providers, tc.obj.Card); len(lines) > 0 {
						entry["context"] = lines
					}
					out = append(out, entry)
				}
				return printJSON(cmd, out)
			}

			reader := bufio.NewReader(cmd.InOrStdin())
			providers := initProviders(cfg)
			return runTriage(context.Background(), untriaged, reader, cmd.OutOrStdout(), providers)
		},
	}
	triageCmd.Flags().Int("limit", 5, "Max contacts to show (-1 for unlimited)")
	rootCmd.AddCommand(triageCmd)
}

func runTriage(ctx context.Context, contacts []triageContact, reader *bufio.Reader, w io.Writer, providers []ContextProvider) error {
	var monthly, quarterly, yearly, skipped, ignored int

	for _, tc := range contacts {
		name := contactName(tc.obj)
		fmt.Fprintf(w, "%s\n", name)
		// Show extra context so the user can identify who this is
		if email := tc.obj.Card.PreferredValue(vcard.FieldEmail); email != "" {
			fmt.Fprintf(w, "  %s\n", email)
		}
		if org := strings.TrimRight(tc.obj.Card.PreferredValue(vcard.FieldOrganization), "; "); org != "" {
			fmt.Fprintf(w, "  %s\n", org)
		}
		if tel := tc.obj.Card.PreferredValue(vcard.FieldTelephone); tel != "" {
			fmt.Fprintf(w, "  %s\n", tel)
		}
		if lines := collectContext(providers, tc.obj.Card); len(lines) > 0 {
			for _, line := range lines {
				fmt.Fprintf(w, "%s\n", line)
			}
		}
		fmt.Fprintf(w, "  [m]onthly  [q]uarterly  [y]early  [s]kip  [i]gnore  [Enter=skip]> ")

		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("reading input: %w", err)
		}
		choice := strings.TrimSpace(strings.ToLower(line))

		switch choice {
		case "m":
			setFrequency(tc.obj.Card, "1m")
			if _, err := tc.client.PutAddressObject(ctx, tc.obj.Path, tc.obj.Card); err != nil {
				return fmt.Errorf("updating %s: %w", name, err)
			}
			monthly++
		case "q":
			setFrequency(tc.obj.Card, "3m")
			if _, err := tc.client.PutAddressObject(ctx, tc.obj.Path, tc.obj.Card); err != nil {
				return fmt.Errorf("updating %s: %w", name, err)
			}
			quarterly++
		case "y":
			setFrequency(tc.obj.Card, "12m")
			if _, err := tc.client.PutAddressObject(ctx, tc.obj.Path, tc.obj.Card); err != nil {
				return fmt.Errorf("updating %s: %w", name, err)
			}
			yearly++
		case "i":
			setIgnored(tc.obj.Card)
			if _, err := tc.client.PutAddressObject(ctx, tc.obj.Path, tc.obj.Card); err != nil {
				return fmt.Errorf("updating %s: %w", name, err)
			}
			ignored++
		default:
			skipped++
		}
	}

	total := monthly + quarterly + yearly + skipped + ignored
	fmt.Fprintf(w, "Triaged %d contacts: %d monthly, %d quarterly, %d yearly, %d skipped, %d ignored\n",
		total, monthly, quarterly, yearly, skipped, ignored)
	return nil
}
