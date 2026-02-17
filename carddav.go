package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
)

func newCardDAVClient(cfg Config) (*carddav.Client, error) {
	endpoint := strings.TrimSuffix(cfg.Endpoint, "/")
	httpClient := webdav.HTTPClientWithBasicAuth(http.DefaultClient, cfg.Username, cfg.Password)
	client, err := carddav.NewClient(httpClient, endpoint)
	if err != nil {
		return nil, fmt.Errorf("connecting to CardDAV: %w", err)
	}
	return client, nil
}

func findAddressBook(ctx context.Context, client *carddav.Client) (*carddav.AddressBook, error) {
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err != nil {
		return nil, fmt.Errorf("finding user principal: %w", err)
	}
	homeSet, err := client.FindAddressBookHomeSet(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("finding address book home set: %w", err)
	}
	books, err := client.FindAddressBooks(ctx, homeSet)
	if err != nil {
		return nil, fmt.Errorf("finding address books: %w", err)
	}
	if len(books) == 0 {
		return nil, fmt.Errorf("no address books found")
	}
	return &books[0], nil
}

func queryAllContacts(ctx context.Context, client *carddav.Client, book *carddav.AddressBook) ([]carddav.AddressObject, error) {
	query := &carddav.AddressBookQuery{
		DataRequest: carddav.AddressDataRequest{
			Props: []string{
				vcard.FieldFormattedName,
				vcard.FieldName,
			},
			AllProp: true,
		},
	}
	objs, err := client.QueryAddressBook(ctx, book.Path, query)
	if err != nil {
		return nil, fmt.Errorf("querying contacts: %w", err)
	}
	return objs, nil
}

func findContactByName(ctx context.Context, client *carddav.Client, book *carddav.AddressBook, name string) (*carddav.AddressObject, error) {
	objs, err := queryAllContacts(ctx, client, book)
	if err != nil {
		return nil, err
	}
	nameLower := strings.ToLower(name)
	for _, obj := range objs {
		fn := obj.Card.PreferredValue(vcard.FieldFormattedName)
		if strings.ToLower(fn) == nameLower {
			return &obj, nil
		}
	}
	return nil, fmt.Errorf("contact %q not found", name)
}

func contactName(obj carddav.AddressObject) string {
	return obj.Card.PreferredValue(vcard.FieldFormattedName)
}
