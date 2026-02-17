package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
)

// ---------------------------------------------------------------------------
// In-memory CardDAV backend (duplicated from e2e_test.go)
// ---------------------------------------------------------------------------

type memBackend struct {
	mu       sync.Mutex
	contacts map[string]carddav.AddressObject
}

func newMemBackend() *memBackend {
	return &memBackend{contacts: make(map[string]carddav.AddressObject)}
}

const (
	principalPath = "/user/"
	homeSetPath   = "/user/addressbooks/"
	abPath        = "/user/addressbooks/default/"

	fieldFrequency   = "X-FRM-FREQUENCY"
	fieldIgnore      = "X-FRM-IGNORE"
	fieldGroup       = "X-FRM-GROUP"
	fieldSnoozeUntil = "X-FRM-SNOOZE-UNTIL"
)

func (b *memBackend) seedContact(name, freq, email, group string, ignore bool, snoozeUntil string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	card := vcard.Card{
		"VERSION":                []*vcard.Field{{Value: "3.0"}},
		vcard.FieldFormattedName: []*vcard.Field{{Value: name}},
	}
	if freq != "" {
		card[fieldFrequency] = []*vcard.Field{{Value: freq}}
	}
	if email != "" {
		card[vcard.FieldEmail] = []*vcard.Field{{Value: email}}
	}
	if group != "" {
		card[fieldGroup] = []*vcard.Field{{Value: group}}
	}
	if ignore {
		card[fieldIgnore] = []*vcard.Field{{Value: "true"}}
	}
	if snoozeUntil != "" {
		card[fieldSnoozeUntil] = []*vcard.Field{{Value: snoozeUntil}}
	}
	p := fmt.Sprintf("%s%s.vcf", abPath, strings.ReplaceAll(strings.ToLower(name), " ", "-"))
	b.contacts[p] = carddav.AddressObject{
		Path:    p,
		ModTime: time.Now(),
		ETag:    fmt.Sprintf("%d", time.Now().UnixNano()),
		Card:    card,
	}
}

func (b *memBackend) CurrentUserPrincipal(_ context.Context) (string, error) {
	return principalPath, nil
}
func (b *memBackend) AddressBookHomeSetPath(_ context.Context) (string, error) {
	return homeSetPath, nil
}
func (b *memBackend) ListAddressBooks(_ context.Context) ([]carddav.AddressBook, error) {
	return []carddav.AddressBook{{Path: abPath, Name: "Contacts"}}, nil
}
func (b *memBackend) GetAddressBook(_ context.Context, _ string) (*carddav.AddressBook, error) {
	return &carddav.AddressBook{Path: abPath, Name: "Contacts"}, nil
}
func (b *memBackend) CreateAddressBook(_ context.Context, _ *carddav.AddressBook) error { return nil }
func (b *memBackend) DeleteAddressBook(_ context.Context, _ string) error               { return nil }
func (b *memBackend) GetAddressObject(_ context.Context, path string, _ *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, ok := b.contacts[path]
	if !ok {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("not found: %s", path))
	}
	return &obj, nil
}
func (b *memBackend) ListAddressObjects(_ context.Context, _ string, _ *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []carddav.AddressObject
	for _, obj := range b.contacts {
		result = append(result, obj)
	}
	return result, nil
}
func (b *memBackend) QueryAddressObjects(_ context.Context, _ string, _ *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []carddav.AddressObject
	for _, obj := range b.contacts {
		result = append(result, obj)
	}
	return result, nil
}
func (b *memBackend) PutAddressObject(_ context.Context, path string, card vcard.Card, _ *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj := carddav.AddressObject{
		Path:    path,
		ModTime: time.Now(),
		ETag:    fmt.Sprintf("%d", time.Now().UnixNano()),
		Card:    card,
	}
	b.contacts[path] = obj
	return &obj, nil
}
func (b *memBackend) DeleteAddressObject(_ context.Context, path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.contacts, path)
	return nil
}

// ---------------------------------------------------------------------------
// Mock JMAP server (duplicated from e2e_test.go)
// ---------------------------------------------------------------------------

type mockMessage struct {
	Subject    string
	ReceivedAt string
}

func newMockJMAPServer(messages map[string][]mockMessage) *httptest.Server {
	mux := http.NewServeMux()
	var apiURL string

	mux.HandleFunc("/jmap/session", func(w http.ResponseWriter, r *http.Request) {
		session := map[string]any{
			"capabilities": map[string]any{
				"urn:ietf:params:jmap:core": map[string]any{},
				"urn:ietf:params:jmap:mail": map[string]any{},
			},
			"accounts":        map[string]any{"a1": map[string]any{"name": "test"}},
			"primaryAccounts": map[string]any{"urn:ietf:params:jmap:mail": "a1"},
			"apiUrl":          apiURL,
			"state":           "0",
			"username":        "test",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(session)
	})

	mux.HandleFunc("/jmap/api", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Calls []json.RawMessage `json:"methodCalls"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		var matchAddrs []string
		var queryCallID string
		for _, raw := range req.Calls {
			var call []json.RawMessage
			json.Unmarshal(raw, &call)
			var method string
			json.Unmarshal(call[0], &method)
			if method == "Email/query" {
				json.Unmarshal(call[2], &queryCallID)
				var args struct {
					Filter json.RawMessage `json:"filter"`
				}
				json.Unmarshal(call[1], &args)
				matchAddrs = extractFilterAddrs(args.Filter)
			}
		}

		var matched []mockMessage
		seen := make(map[string]bool)
		for _, addr := range matchAddrs {
			for _, msg := range messages[addr] {
				key := msg.Subject + msg.ReceivedAt
				if !seen[key] {
					seen[key] = true
					matched = append(matched, msg)
				}
			}
		}

		var ids []string
		var emailList []map[string]any
		for i, msg := range matched {
			id := fmt.Sprintf("msg-%d", i)
			ids = append(ids, id)
			emailList = append(emailList, map[string]any{
				"id":         id,
				"subject":    msg.Subject,
				"receivedAt": msg.ReceivedAt,
			})
		}

		responses := []any{
			[]any{"Email/query", map[string]any{
				"accountId": "a1", "ids": ids, "queryState": "0",
			}, queryCallID},
			[]any{"Email/get", map[string]any{
				"accountId": "a1", "state": "0", "list": emailList,
			}, "1"},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"methodResponses": responses,
			"sessionState":    "0",
		})
	})

	srv := httptest.NewServer(mux)
	apiURL = srv.URL + "/jmap/api"
	return srv
}

func extractFilterAddrs(raw json.RawMessage) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	var addrs []string
	if v, ok := obj["from"]; ok {
		var s string
		json.Unmarshal(v, &s)
		if s != "" {
			addrs = append(addrs, s)
		}
	}
	if v, ok := obj["to"]; ok {
		var s string
		json.Unmarshal(v, &s)
		if s != "" {
			addrs = append(addrs, s)
		}
	}
	if v, ok := obj["conditions"]; ok {
		var conditions []json.RawMessage
		json.Unmarshal(v, &conditions)
		for _, c := range conditions {
			addrs = append(addrs, extractFilterAddrs(c)...)
		}
	}
	return addrs
}

// ---------------------------------------------------------------------------
// Config / LogEntry types (duplicated from main package)
// ---------------------------------------------------------------------------

type Config struct {
	Services []ServiceConfig `json:"services"`
}

type ServiceConfig struct {
	Type            string `json:"type"`
	Endpoint        string `json:"endpoint,omitempty"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	SessionEndpoint string `json:"session_endpoint,omitempty"`
	Token           string `json:"token,omitempty"`
	MaxResults      int    `json:"max_results,omitempty"`
}

type LogEntry struct {
	Contact string    `json:"contact"`
	Path    string    `json:"path,omitempty"`
	Time    time.Time `json:"time"`
	Note    string    `json:"note,omitempty"`
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	// 1. Build the frm binary
	fmt.Println("Building frm binary...")
	rootDir, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot resolve root dir: %v\n", err)
		os.Exit(1)
	}
	binDir, err := os.MkdirTemp("", "frm-demo-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(binDir)

	binaryPath := filepath.Join(binDir, "frm")
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = rootDir
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building binary: %v\n", err)
		os.Exit(1)
	}

	// 2. Start in-memory CardDAV server
	backend := newMemBackend()
	handler := &carddav.Handler{Backend: backend}
	davServer := httptest.NewServer(handler)
	defer davServer.Close()

	// 3. Start mock JMAP server
	now := time.Now().UTC()
	jmapMessages := map[string][]mockMessage{
		"sarah.chen@example.com": {
			{Subject: "Q1 planning sync", ReceivedAt: now.Add(-2 * 24 * time.Hour).Format(time.RFC3339)},
			{Subject: "RE: Conference follow-up", ReceivedAt: now.Add(-12 * 24 * time.Hour).Format(time.RFC3339)},
		},
	}
	jmapServer := newMockJMAPServer(jmapMessages)
	defer jmapServer.Close()

	// 4. Seed contacts
	//    seedContact(name, freq, email, group, ignore, snoozeUntil)

	// Tracked, overdue (never contacted — or contacted long ago via log)
	backend.seedContact("Sarah Chen", "2w", "sarah.chen@example.com", "professional", false, "")
	backend.seedContact("Marcus Rivera", "1m", "", "friends", false, "")
	backend.seedContact("Priya Patel", "3m", "", "family", false, "")

	// Tracked, recently contacted (log entries added below)
	backend.seedContact("Jamie Okafor", "2w", "", "friends", false, "")
	backend.seedContact("Lin Wei", "1m", "", "professional", false, "")

	// Tracked, snoozed
	snoozeDate := now.Add(10 * 24 * time.Hour).Format("2006-01-02")
	backend.seedContact("Dana Kowalski", "2w", "", "", false, snoozeDate)

	// Untracked (for triage)
	backend.seedContact("Noor Abbasi", "", "", "", false, "")
	backend.seedContact("Riley Thompson", "", "", "", false, "")
	backend.seedContact("Kenji Nakamura", "", "", "", false, "")

	// Ignored
	backend.seedContact("Taylor Brooks", "", "", "", true, "")

	// 5. Write config
	configDir, err := os.MkdirTemp("", "frm-demo-config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating config dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(configDir)

	cfg := Config{
		Services: []ServiceConfig{
			{
				Type:     "carddav",
				Endpoint: davServer.URL + "/",
				Username: "test",
				Password: "test",
			},
			{
				Type:            "jmap",
				SessionEndpoint: jmapServer.URL + "/jmap/session",
				Token:           "test-token",
				MaxResults:      5,
			},
		},
	}
	cfgData, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(configDir, "config.json"), cfgData, 0o644)

	// 6. Write pre-seeded log entries
	logEntries := []LogEntry{
		{
			Contact: "Jamie Okafor",
			Path:    abPath + "jamie-okafor.vcf",
			Time:    now.Add(-3 * 24 * time.Hour),
			Note:    "team lunch",
		},
		{
			Contact: "Lin Wei",
			Path:    abPath + "lin-wei.vcf",
			Time:    now.Add(-10 * 24 * time.Hour),
			Note:    "quarterly review call",
		},
		{
			Contact: "Sarah Chen",
			Path:    abPath + "sarah-chen.vcf",
			Time:    now.Add(-18 * 24 * time.Hour),
			Note:    "brainstormed feature priorities for Q2",
		},
		{
			Contact: "Sarah Chen",
			Path:    abPath + "sarah-chen.vcf",
			Time:    now.Add(-35 * 24 * time.Hour),
			Note:    "quick sync on API redesign",
		},
		{
			Contact: "Sarah Chen",
			Path:    abPath + "sarah-chen.vcf",
			Time:    now.Add(-60 * 24 * time.Hour),
			Note:    "intro coffee — discussed her work on distributed systems at Stripe",
		},
	}

	var logBuf []byte
	for _, e := range logEntries {
		line, _ := json.Marshal(e)
		logBuf = append(logBuf, line...)
		logBuf = append(logBuf, '\n')
	}
	os.WriteFile(filepath.Join(configDir, "log.jsonl"), logBuf, 0o644)

	// 7. Create wrapper script so VHS commands look clean ("frm" not a full path)
	wrapperDir, err := os.MkdirTemp("", "frm-demo-wrapper")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating wrapper dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(wrapperDir)

	wrapperPath := filepath.Join(wrapperDir, "frm")
	wrapperScript := fmt.Sprintf("#!/bin/sh\nFRM_CONFIG_DIR=%q exec %q \"$@\"\n", configDir, binaryPath)
	os.WriteFile(wrapperPath, []byte(wrapperScript), 0o755)

	// 8. Run VHS
	tapePath := filepath.Join(rootDir, "demo", "demo.tape")
	fmt.Println("Running VHS...")
	vhs := exec.Command("vhs", tapePath)
	vhs.Dir = rootDir
	// Put our wrapper first on PATH
	vhs.Env = append(os.Environ(),
		"PATH="+wrapperDir+":"+os.Getenv("PATH"),
		"FRM_CONFIG_DIR="+configDir,
	)
	vhs.Stdout = os.Stdout
	vhs.Stderr = os.Stderr
	if err := vhs.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "vhs failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Done! Output: demo.gif")
}
