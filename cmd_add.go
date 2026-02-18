package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/spf13/cobra"
)

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 1
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func init() {
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a new contact to the first CardDAV account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			cfg, err := loadConfig()
			if err != nil {
				return err
			}

			svcs := cfg.carddavServices()
			if len(svcs) == 0 {
				return fmt.Errorf("no CardDAV services configured")
			}

			client, err := newCardDAVClient(svcs[0])
			if err != nil {
				return err
			}

			ctx := context.Background()
			book, err := findAddressBook(ctx, client)
			if err != nil {
				return err
			}

			card := vcard.Card{
				"VERSION":                []*vcard.Field{{Value: "3.0"}},
				vcard.FieldFormattedName: []*vcard.Field{{Value: name}},
			}

			// Split name into given/family for the N field
			parts := strings.SplitN(name, " ", 2)
			given := parts[0]
			family := ""
			if len(parts) > 1 {
				family = parts[1]
			}
			card[vcard.FieldName] = []*vcard.Field{{
				Value: family + ";" + given + ";;;",
			}}

			email, _ := cmd.Flags().GetString("email")
			if email != "" {
				card[vcard.FieldEmail] = []*vcard.Field{{Value: email}}
			}

			phone, _ := cmd.Flags().GetString("phone")
			if phone != "" {
				card[vcard.FieldTelephone] = []*vcard.Field{{Value: phone}}
			}

			org, _ := cmd.Flags().GetString("org")
			if org != "" {
				card[vcard.FieldOrganization] = []*vcard.Field{{Value: org}}
			}

			url, _ := cmd.Flags().GetString("url")
			if url != "" {
				card[vcard.FieldURL] = []*vcard.Field{{Value: url}}
			}

			path := book.Path + newUUID() + ".vcf"
			if _, err := client.PutAddressObject(ctx, path, card); err != nil {
				return fmt.Errorf("creating contact: %w", err)
			}

			fmt.Printf("Added contact %s\n", name)
			return nil
		},
	}
	cmd.Flags().String("email", "", "email address")
	cmd.Flags().String("phone", "", "phone number")
	cmd.Flags().String("org", "", "organization")
	cmd.Flags().String("url", "", "website or social URL")
	rootCmd.AddCommand(cmd)
}
