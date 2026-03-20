package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
	// Try standard CardDAV discovery: principal → home set → address books.
	principal, err := client.FindCurrentUserPrincipal(ctx)
	if err == nil {
		homeSet, err := client.FindAddressBookHomeSet(ctx, principal)
		if err == nil {
			books, err := client.FindAddressBooks(ctx, homeSet)
			if err == nil && len(books) > 0 {
				return &books[0], nil
			}
		}
	}

	// Discovery failed — the endpoint may already be an address book path
	// (e.g. Fastmail's /dav/addressbooks/user/{user}/Default).
	// Try using the endpoint directly.
	books, err := client.FindAddressBooks(ctx, "")
	if err == nil && len(books) > 0 {
		return &books[0], nil
	}

	return nil, fmt.Errorf("could not discover address books (tried standard discovery and direct endpoint)")
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
	var allNames []string
	for _, obj := range objs {
		n := contactName(obj)
		if strings.ToLower(n) == nameLower {
			return &obj, nil
		}
		allNames = append(allNames, n)
	}

	// No exact match. Try fuzzy matching.
	candidates := fuzzyFind(name, allNames)
	if len(candidates) == 1 && candidates[0].distance <= fuzzyMaxDistance {
		for _, obj := range objs {
			if contactName(obj) == candidates[0].name {
				fmt.Fprintf(os.Stderr, "Using closest match: %s\n", candidates[0].name)
				return &obj, nil
			}
		}
	}

	return nil, &fuzzyMatchError{query: name, candidates: candidates}
}

// findContactMulti searches all accounts for a contact by name.
// Returns the matching object and the client it belongs to.
// If no exact match is found, it tries fuzzy matching: substring first,
// then edit distance. A single close match (distance <= 2) is auto-selected
// with a notice on stderr. Multiple candidates produce a suggestion error.
func findContactMulti(cfg Config, name string) (*carddav.AddressObject, *carddav.Client, error) {
	ctx := context.Background()


	// Collect all contacts across accounts for fuzzy fallback.
	type objWithClient struct {
		obj    carddav.AddressObject
		client *carddav.Client
	}
	var allObjs []objWithClient

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
			if normalizedTokensEqual(contactName(obj), name) {
				return &obj, client, nil
			}
			allObjs = append(allObjs, objWithClient{obj: obj, client: client})
		}
	}

	// No exact match. Try fuzzy matching.
	names := make([]string, len(allObjs))
	nameToIdx := make(map[string]int, len(allObjs))
	for i, oc := range allObjs {
		n := contactName(oc.obj)
		names[i] = n
		nameToIdx[n] = i
	}

	candidates := fuzzyFind(name, names)
	if len(candidates) == 1 && candidates[0].distance <= fuzzyMaxDistance {
		idx := nameToIdx[candidates[0].name]
		obj := allObjs[idx].obj
		fmt.Fprintf(os.Stderr, "Using closest match: %s\n", candidates[0].name)
		return &obj, allObjs[idx].client, nil
	}

	return nil, nil, &fuzzyMatchError{query: name, candidates: candidates}
}

// contactMatch holds a matched contact and its client, for multi-account mutations.
type contactMatch struct {
	obj    *carddav.AddressObject
	client *carddav.Client
}

// findAllContactsMulti searches all accounts for contacts matching a name.
// Returns all matches across all accounts.
// Uses the same fuzzy fallback logic as findContactMulti.
func findAllContactsMulti(cfg Config, name string) ([]contactMatch, error) {
	ctx := context.Background()

	var matches []contactMatch

	type objWithClient struct {
		obj    carddav.AddressObject
		client *carddav.Client
	}
	var allObjs []objWithClient

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
			if normalizedTokensEqual(contactName(obj), name) {
				o := obj
				matches = append(matches, contactMatch{obj: &o, client: client})
			}
			allObjs = append(allObjs, objWithClient{obj: obj, client: client})
		}
	}
	if len(matches) > 0 {
		return matches, nil
	}

	// No exact match. Try fuzzy matching.
	names := make([]string, len(allObjs))
	nameToIdx := make(map[string]int, len(allObjs))
	for i, oc := range allObjs {
		n := contactName(oc.obj)
		names[i] = n
		nameToIdx[n] = i
	}

	candidates := fuzzyFind(name, names)
	if len(candidates) == 1 && candidates[0].distance <= fuzzyMaxDistance {
		idx := nameToIdx[candidates[0].name]
		o := allObjs[idx].obj
		fmt.Fprintf(os.Stderr, "Using closest match: %s\n", candidates[0].name)
		return []contactMatch{{obj: &o, client: allObjs[idx].client}}, nil
	}

	return nil, &fuzzyMatchError{query: name, candidates: candidates}
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
