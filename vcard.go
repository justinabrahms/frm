package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

const fieldFrequency = "X-FRM-FREQUENCY"

// parseDuration parses a simple duration string like "2w", "1m", "3d".
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration %q: too short", s)
	}
	suffix := s[len(s)-1]
	numStr := s[:len(s)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	switch suffix {
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case 'm':
		return time.Duration(n) * 30 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration %q: unknown suffix %c (use d, w, or m)", s, suffix)
	}
}

// getFrequency reads X-FRM-FREQUENCY from a vCard.
func getFrequency(card vcard.Card) string {
	return card.PreferredValue(fieldFrequency)
}

// setFrequency sets X-FRM-FREQUENCY on a vCard.
func setFrequency(card vcard.Card, freq string) {
	card[fieldFrequency] = []*vcard.Field{{Value: freq}}
}

// removeFrequency removes X-FRM-FREQUENCY from a vCard.
func removeFrequency(card vcard.Card) {
	delete(card, fieldFrequency)
}
