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

func newCardDAVClient(svc ServiceConfig) (*carddav.Client, error) {
	endpoint := strings.TrimSuffix(svc.Endpoint, "/")
	httpClient := webdav.HTTPClientWithBasicAuth(http.DefaultClient, svc.Username, svc.Password)
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
		if strings.ToLower(contactName(obj)) == nameLower {
			return &obj, nil
		}
	}
	return nil, fmt.Errorf("contact %q not found", name)
}

// findContactMulti searches all accounts for a contact by name.
// Returns the matching object and the client it belongs to.
func findContactMulti(cfg Config, name string) (*carddav.AddressObject, *carddav.Client, error) {
	ctx := context.Background()
	nameLower := strings.ToLower(name)
	for _, svc := range cfg.carddavServices() {
		client, err := newCardDAVClient(svc)
		if err != nil {
			continue
		}
		book, err := findAddressBook(ctx, client)
		if err != nil {
			continue
		}
		objs, err := queryAllContacts(ctx, client, book)
		if err != nil {
			continue
		}
		for _, obj := range objs {
			if strings.ToLower(contactName(obj)) == nameLower {
				return &obj, client, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("contact %q not found", name)
}

// contactMatch holds a matched contact and its client, for multi-account mutations.
type contactMatch struct {
	obj    *carddav.AddressObject
	client *carddav.Client
}

// findAllContactsMulti searches all accounts for contacts matching a name.
// Returns all matches across all accounts.
func findAllContactsMulti(cfg Config, name string) ([]contactMatch, error) {
	ctx := context.Background()
	nameLower := strings.ToLower(name)
	var matches []contactMatch
	for _, svc := range cfg.carddavServices() {
		client, err := newCardDAVClient(svc)
		if err != nil {
			continue
		}
		book, err := findAddressBook(ctx, client)
		if err != nil {
			continue
		}
		objs, err := queryAllContacts(ctx, client, book)
		if err != nil {
			continue
		}
		for _, obj := range objs {
			if strings.ToLower(contactName(obj)) == nameLower {
				o := obj // copy for pointer
				matches = append(matches, contactMatch{obj: &o, client: client})
			}
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("contact %q not found", name)
	}
	return matches, nil
}

func contactName(obj carddav.AddressObject) string {
	if fn := strings.TrimSpace(obj.Card.PreferredValue(vcard.FieldFormattedName)); fn != "" {
		return fn
	}
	// Fall back to structured N field: Family;Given;Additional;Prefix;Suffix
	n := obj.Card.Name()
	if n != nil {
		parts := []string{}
		if n.GivenName != "" {
			parts = append(parts, n.GivenName)
		}
		if n.FamilyName != "" {
			parts = append(parts, n.FamilyName)
		}
		if name := strings.Join(parts, " "); name != "" {
			return name
		}
	}
	return ""
}

// clientAndContacts holds a client and its fetched contacts, used for multi-account iteration.
type clientAndContacts struct {
	client *carddav.Client
	objs   []carddav.AddressObject
}

// allContactsMulti fetches contacts from all configured accounts.
func allContactsMulti(cfg Config) ([]clientAndContacts, error) {
	ctx := context.Background()
	var results []clientAndContacts
	for _, svc := range cfg.carddavServices() {
		client, err := newCardDAVClient(svc)
		if err != nil {
			return nil, err
		}
		book, err := findAddressBook(ctx, client)
		if err != nil {
			return nil, err
		}
		objs, err := queryAllContacts(ctx, client, book)
		if err != nil {
			return nil, err
		}
		results = append(results, clientAndContacts{client: client, objs: objs})
	}
	return results, nil
}
