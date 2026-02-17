package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-vcard"
)

const fieldFrequency = "X-FRM-FREQUENCY"
const fieldIgnore = "X-FRM-IGNORE"
const fieldGroup = "X-FRM-GROUP"
const fieldSnoozeUntil = "X-FRM-SNOOZE-UNTIL"

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

// isIgnored checks if a vCard has X-FRM-IGNORE set to "true".
func isIgnored(card vcard.Card) bool {
	return card.PreferredValue(fieldIgnore) == "true"
}

// setIgnored sets X-FRM-IGNORE to "true" on a vCard.
func setIgnored(card vcard.Card) {
	card[fieldIgnore] = []*vcard.Field{{Value: "true"}}
}

// removeIgnored removes X-FRM-IGNORE from a vCard.
func removeIgnored(card vcard.Card) {
	delete(card, fieldIgnore)
}

// getGroup reads X-FRM-GROUP from a vCard.
func getGroup(card vcard.Card) string {
	return card.PreferredValue(fieldGroup)
}

// setGroup sets X-FRM-GROUP on a vCard.
func setGroup(card vcard.Card, group string) {
	card[fieldGroup] = []*vcard.Field{{Value: group}}
}

// removeGroup removes X-FRM-GROUP from a vCard.
func removeGroup(card vcard.Card) {
	delete(card, fieldGroup)
}

// getSnoozeUntil reads X-FRM-SNOOZE-UNTIL from a vCard as a time.
func getSnoozeUntil(card vcard.Card) (time.Time, bool) {
	v := card.PreferredValue(fieldSnoozeUntil)
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// setSnoozeUntil sets X-FRM-SNOOZE-UNTIL on a vCard.
func setSnoozeUntil(card vcard.Card, t time.Time) {
	card[fieldSnoozeUntil] = []*vcard.Field{{Value: t.Format("2006-01-02")}}
}

// removeSnoozeUntil removes X-FRM-SNOOZE-UNTIL from a vCard.
func removeSnoozeUntil(card vcard.Card) {
	delete(card, fieldSnoozeUntil)
}

// isSnoozed checks if a contact is snoozed until a future date.
func isSnoozed(card vcard.Card) bool {
	t, ok := getSnoozeUntil(card)
	if !ok {
		return false
	}
	return time.Now().Before(t)
}

// parseWhen parses a time string that is either an absolute date (2024-01-15)
// or a relative duration with leading minus (-2w, -3d, -1m).
func parseWhen(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "-") {
		dur, err := parseDuration(s[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid relative time %q: %w", s, err)
		}
		return time.Now().UTC().Add(-dur), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD or relative like -2w", s)
	}
	return t.UTC(), nil
}

// parseUntil parses a target date that is either absolute (2026-04-01)
// or a relative duration from now (2m, 6w).
func parseUntil(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	// Try absolute date first
	t, err := time.Parse("2006-01-02", s)
	if err == nil {
		return t, nil
	}
	// Try relative duration
	dur, err := parseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: use YYYY-MM-DD or relative like 2m", s)
	}
	return time.Now().Add(dur), nil
}
