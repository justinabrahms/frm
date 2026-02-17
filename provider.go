package main

import (
	"fmt"
	"os"

	"github.com/emersion/go-vcard"
)

// ContextProvider returns lines of context about a contact.
type ContextProvider interface {
	Name() string
	GetContext(card vcard.Card) ([]string, error)
}

// initProviders creates providers based on config.
func initProviders(cfg Config) []ContextProvider {
	var providers []ContextProvider
	for _, svc := range cfg.jmapServices() {
		p, err := newJMAPProvider(svc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jmap provider: %v\n", err)
		} else {
			providers = append(providers, p)
		}
	}
	return providers
}

// collectContext gathers context from all providers for a given card.
func collectContext(providers []ContextProvider, card vcard.Card) []string {
	var all []string
	for _, p := range providers {
		lines, err := p.GetContext(card)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", p.Name(), err)
			continue
		}
		if len(lines) == 0 {
			continue
		}
		all = append(all, p.Name()+":")
		all = append(all, lines...)
	}
	return all
}
