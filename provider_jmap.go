package main

import (
	"fmt"

	"git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/mail"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
	"github.com/emersion/go-vcard"
)

type jmapProvider struct {
	client    *jmap.Client
	accountID jmap.ID
	maxResults int
}

func newJMAPProvider(svc ServiceConfig) (*jmapProvider, error) {
	client := &jmap.Client{
		SessionEndpoint: svc.SessionEndpoint,
	}
	client.WithAccessToken(svc.Token)

	if err := client.Authenticate(); err != nil {
		return nil, fmt.Errorf("authenticating: %w", err)
	}

	accountID, ok := client.Session.PrimaryAccounts[mail.URI]
	if !ok {
		return nil, fmt.Errorf("no mail account found")
	}

	maxResults := svc.MaxResults
	if maxResults <= 0 {
		maxResults = 3
	}

	return &jmapProvider{
		client:     client,
		accountID:  accountID,
		maxResults: maxResults,
	}, nil
}

func (p *jmapProvider) Name() string { return "Recent emails" }

func (p *jmapProvider) GetContext(card vcard.Card) ([]string, error) {
	addrs := extractEmails(card)
	if len(addrs) == 0 {
		return nil, nil
	}

	filter := buildEmailFilter(addrs)

	req := &jmap.Request{}
	queryID := req.Invoke(&email.Query{
		Account: p.accountID,
		Filter:  filter,
		Sort: []*email.SortComparator{
			{Property: "receivedAt", IsAscending: false},
		},
		Limit: uint64(p.maxResults),
	})
	req.Invoke(&email.Get{
		Account:    p.accountID,
		Properties: []string{"subject", "receivedAt"},
		ReferenceIDs: &jmap.ResultReference{
			ResultOf: queryID,
			Name:     "Email/query",
			Path:     "/ids",
		},
	})

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying emails: %w", err)
	}

	var lines []string
	for _, inv := range resp.Responses {
		if getResp, ok := inv.Args.(*email.GetResponse); ok {
			for _, msg := range getResp.List {
				date := ""
				if msg.ReceivedAt != nil {
					date = msg.ReceivedAt.Format("2006-01-02")
				}
				lines = append(lines, fmt.Sprintf("  %s (%s)", msg.Subject, date))
			}
		}
	}
	return lines, nil
}

func extractEmails(card vcard.Card) []string {
	fields := card[vcard.FieldEmail]
	if len(fields) == 0 {
		return nil
	}
	var addrs []string
	for _, f := range fields {
		if f.Value != "" {
			addrs = append(addrs, f.Value)
		}
	}
	return addrs
}

func buildEmailFilter(addrs []string) email.Filter {
	var conditions []email.Filter
	for _, addr := range addrs {
		conditions = append(conditions, &email.FilterCondition{From: addr})
		conditions = append(conditions, &email.FilterCondition{To: addr})
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	return &email.FilterOperator{
		Operator:   jmap.OperatorOR,
		Conditions: conditions,
	}
}
