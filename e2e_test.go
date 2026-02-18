package main

import (
	"bytes"
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
	"testing"
	"time"

	"github.com/emersion/go-vcard"
	"github.com/emersion/go-webdav"
	"github.com/emersion/go-webdav/carddav"
)

// ---------------------------------------------------------------------------
// In-memory CardDAV backend
// ---------------------------------------------------------------------------

type memBackend struct {
	mu       sync.Mutex
	contacts map[string]carddav.AddressObject // path -> object
}

func newMemBackend() *memBackend {
	return &memBackend{contacts: make(map[string]carddav.AddressObject)}
}

const (
	principalPath = "/user/"
	homeSetPath   = "/user/addressbooks/"
	abPath        = "/user/addressbooks/default/"
)

func (b *memBackend) seedContact(name, freq string) {
	b.seedContactWithEmail(name, freq, "")
}

func (b *memBackend) seedContactWithEmail(name, freq, email string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	card := vcard.Card{
		"VERSION": []*vcard.Field{{Value: "3.0"}},
		vcard.FieldFormattedName: []*vcard.Field{{Value: name}},
	}
	if freq != "" {
		card[fieldFrequency] = []*vcard.Field{{Value: freq}}
	}
	if email != "" {
		card[vcard.FieldEmail] = []*vcard.Field{{Value: email}}
	}
	p := fmt.Sprintf("%s%s.vcf", abPath, strings.ReplaceAll(strings.ToLower(name), " ", "-"))
	b.contacts[p] = carddav.AddressObject{
		Path:    p,
		ModTime: time.Now(),
		ETag:    fmt.Sprintf("%d", time.Now().UnixNano()),
		Card:    card,
	}
}

func (b *memBackend) CurrentUserPrincipal(ctx context.Context) (string, error) {
	return principalPath, nil
}

func (b *memBackend) AddressBookHomeSetPath(ctx context.Context) (string, error) {
	return homeSetPath, nil
}

func (b *memBackend) ListAddressBooks(ctx context.Context) ([]carddav.AddressBook, error) {
	return []carddav.AddressBook{{
		Path: abPath,
		Name: "Contacts",
	}}, nil
}

func (b *memBackend) GetAddressBook(ctx context.Context, path string) (*carddav.AddressBook, error) {
	return &carddav.AddressBook{
		Path: abPath,
		Name: "Contacts",
	}, nil
}

func (b *memBackend) CreateAddressBook(ctx context.Context, ab *carddav.AddressBook) error {
	return nil
}

func (b *memBackend) DeleteAddressBook(ctx context.Context, path string) error {
	return nil
}

func (b *memBackend) GetAddressObject(ctx context.Context, path string, req *carddav.AddressDataRequest) (*carddav.AddressObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	obj, ok := b.contacts[path]
	if !ok {
		return nil, webdav.NewHTTPError(http.StatusNotFound, fmt.Errorf("not found: %s", path))
	}
	return &obj, nil
}

func (b *memBackend) ListAddressObjects(ctx context.Context, path string, req *carddav.AddressDataRequest) ([]carddav.AddressObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []carddav.AddressObject
	for _, obj := range b.contacts {
		result = append(result, obj)
	}
	return result, nil
}

func (b *memBackend) QueryAddressObjects(ctx context.Context, path string, query *carddav.AddressBookQuery) ([]carddav.AddressObject, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var result []carddav.AddressObject
	for _, obj := range b.contacts {
		result = append(result, obj)
	}
	return result, nil
}

func (b *memBackend) PutAddressObject(ctx context.Context, path string, card vcard.Card, opts *carddav.PutAddressObjectOptions) (*carddav.AddressObject, error) {
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

func (b *memBackend) DeleteAddressObject(ctx context.Context, path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.contacts, path)
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

var binaryPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "frm-test-bin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	binaryPath = filepath.Join(tmp, "frm")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building binary: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

type testEnv struct {
	server    *httptest.Server
	backend   *memBackend
	configDir string
}

func setupTest(t *testing.T) *testEnv {
	t.Helper()

	backend := newMemBackend()
	handler := &carddav.Handler{Backend: backend}
	server := httptest.NewServer(handler)

	configDir := t.TempDir()
	cfg := Config{
		Services: []ServiceConfig{{
			Type:     "carddav",
			Endpoint: server.URL + "/",
			Username: "test",
			Password: "test",
		}},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshaling config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	t.Cleanup(func() { server.Close() })

	return &testEnv{
		server:    server,
		backend:   backend,
		configDir: configDir,
	}
}

func (e *testEnv) run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return e.runWithStdin(t, nil, args...)
}

func (e *testEnv) runWithStdin(t *testing.T, stdin *strings.Reader, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "FRM_CONFIG_DIR="+e.configDir)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func (e *testEnv) getContactCard(name string) vcard.Card {
	e.backend.mu.Lock()
	defer e.backend.mu.Unlock()
	for _, obj := range e.backend.contacts {
		if obj.Card.PreferredValue(vcard.FieldFormattedName) == name {
			return obj.Card
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// E2E tests
// ---------------------------------------------------------------------------

func TestE2E_List(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")  // tracked
	env.backend.seedContact("Bob", "")       // untracked
	env.backend.seedContact("Charlie", "1m") // tracked

	// Set a group on Alice
	env.run(t, "group", "set", "Alice", "friends")

	// Default: only tracked contacts, with frequency and group in table
	stdout, _, err := env.run(t, "list")
	if err != nil {
		t.Fatalf("frm list failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 { // header + 2 contacts
		t.Fatalf("expected 3 lines (header + 2 contacts), got %d: %q", len(lines), stdout)
	}
	if !strings.Contains(lines[0], "NAME") || !strings.Contains(lines[0], "FREQ") {
		t.Errorf("expected table header, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "Alice") || !strings.Contains(lines[1], "2w") {
		t.Errorf("expected Alice with frequency, got: %s", lines[1])
	}
	if !strings.Contains(lines[1], "friends") {
		t.Errorf("expected friends group, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "Charlie") || !strings.Contains(lines[2], "1m") {
		t.Errorf("expected Charlie with frequency, got: %s", lines[2])
	}
	if strings.Contains(stdout, "Bob") {
		t.Error("untracked Bob should not appear in list")
	}

	// --all: everyone
	stdout, _, err = env.run(t, "list", "--all")
	if err != nil {
		t.Fatalf("frm list --all failed: %v", err)
	}
	lines = strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 { // header + 3 contacts
		t.Fatalf("expected 4 lines (header + 3 contacts) with --all, got %d: %q", len(lines), stdout)
	}

	// Ignored contacts excluded from default list
	env.run(t, "ignore", "Alice")
	stdout, _, err = env.run(t, "list")
	if err != nil {
		t.Fatalf("frm list failed: %v", err)
	}
	if strings.Contains(stdout, "Alice") {
		t.Error("ignored Alice should not appear in list")
	}

	// JSON output includes structured data
	env.run(t, "unignore", "Alice")
	stdout, _, err = env.run(t, "list", "--json")
	if err != nil {
		t.Fatalf("frm list --json failed: %v", err)
	}
	if !strings.Contains(stdout, `"frequency"`) {
		t.Errorf("expected frequency in JSON, got: %s", stdout)
	}
	if !strings.Contains(stdout, `"due_in_days"`) {
		t.Errorf("expected due_in_days in JSON, got: %s", stdout)
	}
}

func TestE2E_Contacts(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Charlie", "")
	env.backend.seedContact("Alice", "")
	env.backend.seedContact("Bob", "")

	stdout, stderr, err := env.run(t, "contacts")
	if err != nil {
		t.Fatalf("frm contacts failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), stdout)
	}
	if lines[0] != "Alice" || lines[1] != "Bob" || lines[2] != "Charlie" {
		t.Errorf("expected alphabetical order [Alice Bob Charlie], got %v", lines)
	}
}

func TestE2E_Track(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "")

	stdout, _, err := env.run(t, "track", "Alice", "--every", "2w")
	if err != nil {
		t.Fatalf("frm track failed: %v", err)
	}
	if !strings.Contains(stdout, "Tracking Alice every 2w") {
		t.Errorf("unexpected output: %q", stdout)
	}

	card := env.getContactCard("Alice")
	if card == nil {
		t.Fatal("Alice not found in backend")
	}
	freq := card.PreferredValue(fieldFrequency)
	if freq != "2w" {
		t.Errorf("expected frequency 2w, got %q", freq)
	}
}

func TestE2E_Untrack(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")

	stdout, _, err := env.run(t, "untrack", "Alice")
	if err != nil {
		t.Fatalf("frm untrack failed: %v", err)
	}
	if !strings.Contains(stdout, "Stopped tracking Alice") {
		t.Errorf("unexpected output: %q", stdout)
	}

	card := env.getContactCard("Alice")
	if card == nil {
		t.Fatal("Alice not found in backend")
	}
	freq := card.PreferredValue(fieldFrequency)
	if freq != "" {
		t.Errorf("expected empty frequency, got %q", freq)
	}
}

func TestE2E_Log(t *testing.T) {
	env := setupTest(t)

	stdout, _, err := env.run(t, "log", "Alice", "--note", "coffee")
	if err != nil {
		t.Fatalf("frm log failed: %v", err)
	}
	if !strings.Contains(stdout, "Logged interaction with Alice") {
		t.Errorf("unexpected output: %q", stdout)
	}

	data, err := os.ReadFile(filepath.Join(env.configDir, "log.jsonl"))
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parsing log entry: %v", err)
	}
	if entry.Contact != "Alice" || entry.Note != "coffee" {
		t.Errorf("unexpected log entry: %+v", entry)
	}
}

func TestE2E_Check(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")
	env.backend.seedContact("Bob", "1w")
	env.backend.seedContact("Charlie", "") // untracked

	// Log a recent interaction with Alice (not overdue)
	recentEntry := LogEntry{
		Contact: "Alice",
		Time:    time.Now().UTC().Add(-24 * time.Hour), // 1 day ago
		Note:    "lunch",
	}
	data, _ := json.Marshal(recentEntry)
	logPath := filepath.Join(env.configDir, "log.jsonl")
	if err := os.WriteFile(logPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("writing log: %v", err)
	}

	// Bob has tracking but no log entries -> overdue
	stdout, _, err := env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}

	if strings.Contains(stdout, "Alice") {
		t.Errorf("Alice should NOT be overdue (contacted 1 day ago, tracked every 2w)")
	}
	if !strings.Contains(stdout, "Bob") {
		t.Errorf("Bob should be overdue (tracked every 1w, never contacted)")
	}
	if strings.Contains(stdout, "Charlie") {
		t.Errorf("Charlie should NOT appear (untracked)")
	}
}

func TestE2E_Triage(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "")
	env.backend.seedContact("Bob", "")
	env.backend.seedContact("Charlie", "")

	// Alphabetical order: Alice, Bob, Charlie
	// m=monthly for Alice, q=quarterly for Bob, i=ignore for Charlie
	stdin := strings.NewReader("m\nq\ni\n")
	stdout, stderr, err := env.runWithStdin(t, stdin, "triage")
	if err != nil {
		t.Fatalf("frm triage failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	// Verify summary
	if !strings.Contains(stdout, "1 monthly") {
		t.Errorf("expected 1 monthly in summary, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 quarterly") {
		t.Errorf("expected 1 quarterly in summary, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 ignored") {
		t.Errorf("expected 1 ignored in summary, got: %s", stdout)
	}

	// Verify Alice got monthly (1m)
	aliceCard := env.getContactCard("Alice")
	if aliceCard == nil {
		t.Fatal("Alice not found")
	}
	if freq := aliceCard.PreferredValue(fieldFrequency); freq != "1m" {
		t.Errorf("expected Alice frequency 1m, got %q", freq)
	}

	// Verify Bob got quarterly (3m)
	bobCard := env.getContactCard("Bob")
	if bobCard == nil {
		t.Fatal("Bob not found")
	}
	if freq := bobCard.PreferredValue(fieldFrequency); freq != "3m" {
		t.Errorf("expected Bob frequency 3m, got %q", freq)
	}

	// Verify Charlie is ignored
	charlieCard := env.getContactCard("Charlie")
	if charlieCard == nil {
		t.Fatal("Charlie not found")
	}
	if charlieCard.PreferredValue(fieldIgnore) != "true" {
		t.Errorf("expected Charlie to be ignored")
	}

	// Verify ignored contact doesn't appear in check
	checkOut, _, err := env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}
	if strings.Contains(checkOut, "Charlie") {
		t.Errorf("ignored contact Charlie should not appear in check output")
	}
}

func TestE2E_History(t *testing.T) {
	env := setupTest(t)

	// Log two interactions
	env.run(t, "log", "Alice", "--note", "coffee")
	env.run(t, "log", "Alice", "--note", "lunch")
	env.run(t, "log", "Bob", "--note", "call")

	stdout, _, err := env.run(t, "history", "Alice")
	if err != nil {
		t.Fatalf("frm history failed: %v", err)
	}
	if !strings.Contains(stdout, "coffee") {
		t.Errorf("expected coffee in history, got: %s", stdout)
	}
	if !strings.Contains(stdout, "lunch") {
		t.Errorf("expected lunch in history, got: %s", stdout)
	}
	if strings.Contains(stdout, "call") {
		t.Errorf("Bob's interaction should not appear in Alice's history")
	}

	// No history
	stdout, _, err = env.run(t, "history", "Nobody")
	if err != nil {
		t.Fatalf("frm history failed: %v", err)
	}
	if !strings.Contains(stdout, "No interactions") {
		t.Errorf("expected no interactions message, got: %s", stdout)
	}
}

func TestE2E_Context(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")

	// Log an interaction
	env.run(t, "log", "Alice", "--note", "dinner")

	stdout, _, err := env.run(t, "context", "Alice")
	if err != nil {
		t.Fatalf("frm context failed: %v", err)
	}
	if !strings.Contains(stdout, "Alice") {
		t.Errorf("expected Alice in context, got: %s", stdout)
	}
	if !strings.Contains(stdout, "every 2w") {
		t.Errorf("expected frequency in context, got: %s", stdout)
	}
	if !strings.Contains(stdout, "dinner") {
		t.Errorf("expected last note in context, got: %s", stdout)
	}
}

func TestE2E_Unignore(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "")

	// Triage to ignore Alice
	stdin := strings.NewReader("i\n")
	_, _, err := env.runWithStdin(t, stdin, "triage")
	if err != nil {
		t.Fatalf("frm triage failed: %v", err)
	}

	// Verify ignored
	card := env.getContactCard("Alice")
	if card.PreferredValue(fieldIgnore) != "true" {
		t.Fatal("Alice should be ignored")
	}

	// Unignore
	stdout, _, err := env.run(t, "unignore", "Alice")
	if err != nil {
		t.Fatalf("frm unignore failed: %v", err)
	}
	if !strings.Contains(stdout, "Unignored Alice") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Verify not ignored
	card = env.getContactCard("Alice")
	if card.PreferredValue(fieldIgnore) != "" {
		t.Errorf("Alice should no longer be ignored")
	}
}

func TestE2E_Stats(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")
	env.backend.seedContact("Bob", "1m")
	env.backend.seedContact("Charlie", "")

	env.run(t, "log", "Alice", "--note", "coffee")
	env.run(t, "log", "Alice", "--note", "lunch")
	env.run(t, "log", "Bob", "--note", "call")

	stdout, _, err := env.run(t, "stats")
	if err != nil {
		t.Fatalf("frm stats failed: %v", err)
	}
	if !strings.Contains(stdout, "Total contacts:  3") {
		t.Errorf("expected 3 total contacts, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Tracked:         2") {
		t.Errorf("expected 2 tracked, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Most contacted:  Alice") {
		t.Errorf("expected Alice as most contacted, got: %s", stdout)
	}
}

func TestE2E_Group(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")
	env.backend.seedContact("Bob", "1m")

	// Set groups
	stdout, _, err := env.run(t, "group", "set", "Alice", "friends")
	if err != nil {
		t.Fatalf("frm group set failed: %v", err)
	}
	if !strings.Contains(stdout, "Set Alice group to friends") {
		t.Errorf("unexpected output: %s", stdout)
	}

	env.run(t, "group", "set", "Bob", "professional")

	// List all groups
	stdout, _, err = env.run(t, "group", "list")
	if err != nil {
		t.Fatalf("frm group list failed: %v", err)
	}
	if !strings.Contains(stdout, "friends") {
		t.Errorf("expected friends in groups, got: %s", stdout)
	}
	if !strings.Contains(stdout, "professional") {
		t.Errorf("expected professional in groups, got: %s", stdout)
	}

	// List contacts in group
	stdout, _, err = env.run(t, "group", "list", "friends")
	if err != nil {
		t.Fatalf("frm group list friends failed: %v", err)
	}
	if !strings.Contains(stdout, "Alice") {
		t.Errorf("expected Alice in friends group, got: %s", stdout)
	}
	if strings.Contains(stdout, "Bob") {
		t.Errorf("Bob should not be in friends group")
	}

	// Unset group
	stdout, _, err = env.run(t, "group", "unset", "Alice")
	if err != nil {
		t.Fatalf("frm group unset failed: %v", err)
	}
	if !strings.Contains(stdout, "Removed group from Alice") {
		t.Errorf("unexpected output: %s", stdout)
	}

	card := env.getContactCard("Alice")
	if card.PreferredValue(fieldGroup) != "" {
		t.Errorf("Alice should have no group")
	}
}

func TestE2E_JSON(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")
	env.backend.seedContact("Bob", "")

	// contacts --json
	stdout, _, err := env.run(t, "contacts", "--json")
	if err != nil {
		t.Fatalf("frm contacts --json failed: %v", err)
	}
	if !strings.Contains(stdout, `"Alice"`) {
		t.Errorf("expected Alice in JSON output, got: %s", stdout)
	}

	// check --json
	stdout, _, err = env.run(t, "check", "--json")
	if err != nil {
		t.Fatalf("frm check --json failed: %v", err)
	}
	if !strings.Contains(stdout, `"name"`) {
		t.Errorf("expected JSON object with name field, got: %s", stdout)
	}

	// context --json
	stdout, _, err = env.run(t, "context", "Alice", "--json")
	if err != nil {
		t.Fatalf("frm context --json failed: %v", err)
	}
	if !strings.Contains(stdout, `"frequency"`) {
		t.Errorf("expected frequency in JSON context, got: %s", stdout)
	}
}

func TestE2E_LogPathNormalization(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")

	// Log an interaction — should resolve path
	env.run(t, "log", "Alice", "--note", "coffee")

	// Read the log file and verify path is set
	data, err := os.ReadFile(filepath.Join(env.configDir, "log.jsonl"))
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parsing log entry: %v", err)
	}
	if entry.Path == "" {
		t.Error("expected path in log entry for name normalization")
	}
	if entry.Contact != "Alice" {
		t.Errorf("expected contact name Alice, got %q", entry.Contact)
	}
}

// ---------------------------------------------------------------------------
// Mock JMAP server
// ---------------------------------------------------------------------------

type mockMessage struct {
	Subject    string
	ReceivedAt string // RFC3339
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
			"accounts": map[string]any{
				"a1": map[string]any{
					"name": "test",
				},
			},
			"primaryAccounts": map[string]any{
				"urn:ietf:params:jmap:mail": "a1",
			},
			"apiUrl":   apiURL,
			"state":    "0",
			"username": "test",
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

		// Collect email addresses from the query filter
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

		// Find matching messages
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

		// Build email IDs and email objects
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
				"accountId":  "a1",
				"ids":        ids,
				"queryState": "0",
			}, queryCallID},
			[]any{"Email/get", map[string]any{
				"accountId": "a1",
				"state":     "0",
				"list":      emailList,
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
	// Check for direct from/to
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
	// Check for operator (OR/AND) with conditions
	if v, ok := obj["conditions"]; ok {
		var conditions []json.RawMessage
		json.Unmarshal(v, &conditions)
		for _, c := range conditions {
			addrs = append(addrs, extractFilterAddrs(c)...)
		}
	}
	return addrs
}

func setupTestWithJMAP(t *testing.T, messages map[string][]mockMessage) *testEnv {
	t.Helper()

	jmapServer := newMockJMAPServer(messages)

	backend := newMemBackend()
	handler := &carddav.Handler{Backend: backend}
	server := httptest.NewServer(handler)

	configDir := t.TempDir()
	cfg := Config{
		Services: []ServiceConfig{
			{
				Type:     "carddav",
				Endpoint: server.URL + "/",
				Username: "test",
				Password: "test",
			},
			{
				Type:            "jmap",
				SessionEndpoint: jmapServer.URL + "/jmap/session",
				Token:           "test-token",
				MaxResults:      3,
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshaling config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	t.Cleanup(func() {
		server.Close()
		jmapServer.Close()
	})

	return &testEnv{
		server:    server,
		backend:   backend,
		configDir: configDir,
	}
}

func TestE2E_ContextWithJMAP(t *testing.T) {
	messages := map[string][]mockMessage{
		"alice@example.com": {
			{Subject: "Weekend plans?", ReceivedAt: "2024-01-15T10:00:00Z"},
			{Subject: "RE: Project collaboration", ReceivedAt: "2024-01-12T09:00:00Z"},
			{Subject: "Thanks for the recommendation", ReceivedAt: "2024-01-08T14:00:00Z"},
		},
	}
	env := setupTestWithJMAP(t, messages)
	env.backend.seedContactWithEmail("Alice", "2w", "alice@example.com")

	stdout, stderr, err := env.run(t, "context", "Alice")
	if err != nil {
		t.Fatalf("frm context failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "Weekend plans?") {
		t.Errorf("expected 'Weekend plans?' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "RE: Project collaboration") {
		t.Errorf("expected 'RE: Project collaboration' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Recent emails:") {
		t.Errorf("expected 'Recent emails:' header in output, got: %s", stdout)
	}
}

func TestE2E_TriageWithJMAP(t *testing.T) {
	messages := map[string][]mockMessage{
		"alice@example.com": {
			{Subject: "Lunch tomorrow?", ReceivedAt: "2024-01-15T10:00:00Z"},
		},
	}
	env := setupTestWithJMAP(t, messages)
	env.backend.seedContactWithEmail("Alice", "", "alice@example.com")

	stdin := strings.NewReader("s\n")
	stdout, stderr, err := env.runWithStdin(t, stdin, "triage")
	if err != nil {
		t.Fatalf("frm triage failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "Lunch tomorrow?") {
		t.Errorf("expected 'Lunch tomorrow?' in triage output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Recent emails:") {
		t.Errorf("expected 'Recent emails:' header in triage output, got: %s", stdout)
	}
}

func TestE2E_TriageJSON(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")                     // has frequency — should be excluded
	env.backend.seedContactWithEmail("Bob", "", "bob@test.com") // untriaged — should appear
	// Seed an ignored contact
	env.backend.seedContact("Charlie", "")
	card := env.getContactCard("Charlie")
	card[fieldIgnore] = []*vcard.Field{{Value: "true"}}
	env.backend.mu.Lock()
	for p, obj := range env.backend.contacts {
		if obj.Card.PreferredValue(vcard.FieldFormattedName) == "Charlie" {
			obj.Card = card
			env.backend.contacts[p] = obj
		}
	}
	env.backend.mu.Unlock()

	stdout, stderr, err := env.run(t, "triage", "--json")
	if err != nil {
		t.Fatalf("frm triage --json failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	var result []map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 untriaged contact, got %d: %s", len(result), stdout)
	}
	if result[0]["name"] != "Bob" {
		t.Errorf("expected Bob, got %v", result[0]["name"])
	}
	if result[0]["email"] != "bob@test.com" {
		t.Errorf("expected bob@test.com, got %v", result[0]["email"])
	}
}

func TestE2E_Ignore(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")

	stdout, _, err := env.run(t, "ignore", "Alice")
	if err != nil {
		t.Fatalf("frm ignore failed: %v", err)
	}
	if !strings.Contains(stdout, "Ignored Alice") {
		t.Errorf("unexpected output: %s", stdout)
	}

	card := env.getContactCard("Alice")
	if card.PreferredValue(fieldIgnore) != "true" {
		t.Error("expected Alice to be ignored")
	}

	// Verify ignored contact doesn't appear in check
	checkOut, _, err := env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}
	if strings.Contains(checkOut, "Alice") {
		t.Error("ignored contact Alice should not appear in check output")
	}

	// Running ignore again should say already ignored
	stdout, _, err = env.run(t, "ignore", "Alice")
	if err != nil {
		t.Fatalf("frm ignore (second) failed: %v", err)
	}
	if !strings.Contains(stdout, "already ignored") {
		t.Errorf("expected 'already ignored', got: %s", stdout)
	}
}

func TestE2E_LogWhen(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")

	// Log with absolute date
	stdout, _, err := env.run(t, "log", "Alice", "--note", "coffee", "--when", "2025-06-15")
	if err != nil {
		t.Fatalf("frm log --when failed: %v", err)
	}
	if !strings.Contains(stdout, "Logged interaction with Alice") {
		t.Errorf("unexpected output: %s", stdout)
	}

	data, err := os.ReadFile(filepath.Join(env.configDir, "log.jsonl"))
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}
	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("parsing log: %v", err)
	}
	if entry.Time.Format("2006-01-02") != "2025-06-15" {
		t.Errorf("expected 2025-06-15, got %s", entry.Time.Format("2006-01-02"))
	}

	// Log with relative date
	stdout, _, err = env.run(t, "log", "Alice", "--note", "lunch", "--when", "-1w")
	if err != nil {
		t.Fatalf("frm log --when relative failed: %v", err)
	}

	// Read second entry
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Re-read since we appended
	data, _ = os.ReadFile(filepath.Join(env.configDir, "log.jsonl"))
	lines = strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected 2 log entries, got %d", len(lines))
	}
	var entry2 LogEntry
	json.Unmarshal([]byte(lines[1]), &entry2)
	daysSince := int(time.Since(entry2.Time).Hours() / 24)
	if daysSince < 5 || daysSince > 9 {
		t.Errorf("expected ~7 days ago, got %d days ago", daysSince)
	}
}

func TestE2E_Snooze(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Tracy", "3m")

	// Tracy is overdue (never contacted)
	stdout, _, err := env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}
	if !strings.Contains(stdout, "Tracy") {
		t.Errorf("Tracy should be overdue, got: %s", stdout)
	}

	// Snooze Tracy for 2 months
	stdout, _, err = env.run(t, "snooze", "Tracy", "--until", "2m")
	if err != nil {
		t.Fatalf("frm snooze failed: %v", err)
	}
	if !strings.Contains(stdout, "Snoozed Tracy until") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Tracy should not appear in check now
	stdout, _, err = env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}
	if strings.Contains(stdout, "Tracy") {
		t.Errorf("snoozed Tracy should not appear in check, got: %s", stdout)
	}

	// Verify vCard field is set
	card := env.getContactCard("Tracy")
	if card.PreferredValue(fieldSnoozeUntil) == "" {
		t.Error("expected X-FRM-SNOOZE-UNTIL to be set")
	}

	// Unsnooze
	stdout, _, err = env.run(t, "unsnooze", "Tracy")
	if err != nil {
		t.Fatalf("frm unsnooze failed: %v", err)
	}
	if !strings.Contains(stdout, "Unsnoozed Tracy") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Tracy should be back in check
	stdout, _, err = env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}
	if !strings.Contains(stdout, "Tracy") {
		t.Errorf("unsnoozed Tracy should appear in check, got: %s", stdout)
	}
}

func TestE2E_SnoozeAbsoluteDate(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "1w")

	// Snooze with absolute future date
	stdout, _, err := env.run(t, "snooze", "Alice", "--until", "2099-12-31")
	if err != nil {
		t.Fatalf("frm snooze failed: %v", err)
	}
	if !strings.Contains(stdout, "Snoozed Alice until 2099-12-31") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Should not appear in check
	stdout, _, err = env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}
	if strings.Contains(stdout, "Alice") {
		t.Errorf("snoozed Alice should not appear in check")
	}
}

func TestE2E_Spread(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "1m")
	env.backend.seedContact("Bob", "1m")
	env.backend.seedContact("Charlie", "1m")
	env.backend.seedContact("Dana", "2w") // different frequency

	// All 4 are never-contacted and tracked → all overdue
	stdout, _, err := env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check failed: %v", err)
	}
	for _, name := range []string{"Alice", "Bob", "Charlie", "Dana"} {
		if !strings.Contains(stdout, name) {
			t.Errorf("expected %s in check output before spread", name)
		}
	}

	// Default is dry run
	stdout, _, err = env.run(t, "spread")
	if err != nil {
		t.Fatalf("frm spread (dry run) failed: %v", err)
	}
	if !strings.Contains(stdout, "Dry run") {
		t.Errorf("expected dry run message, got: %s", stdout)
	}
	// Verify no snooze was actually set
	for _, name := range []string{"Alice", "Bob", "Charlie", "Dana"} {
		card := env.getContactCard(name)
		if card.PreferredValue(fieldSnoozeUntil) != "" {
			t.Errorf("%s should not be snoozed after dry run", name)
		}
	}

	// Real run with --apply
	stdout, _, err = env.run(t, "spread", "--apply")
	if err != nil {
		t.Fatalf("frm spread failed: %v", err)
	}
	if !strings.Contains(stdout, "Spread 4 contacts") {
		t.Errorf("expected spread summary, got: %s", stdout)
	}

	// First contact in each group (alphabetically) should be due now (snoozed until today)
	// Others should be snoozed into the future
	// Check that at least some contacts are now snoozed
	snoozed := 0
	for _, name := range []string{"Alice", "Bob", "Charlie", "Dana"} {
		card := env.getContactCard(name)
		if card.PreferredValue(fieldSnoozeUntil) != "" {
			snoozed++
		}
	}
	if snoozed == 0 {
		t.Error("expected at least some contacts to be snoozed after spread")
	}

	// Check should now show fewer overdue contacts
	stdout, _, err = env.run(t, "check")
	if err != nil {
		t.Fatalf("frm check after spread failed: %v", err)
	}
	// At minimum, the snoozed ones should be hidden
	if strings.Contains(stdout, "Bob") && strings.Contains(stdout, "Charlie") {
		t.Errorf("expected some monthly contacts to be snoozed out of check, got: %s", stdout)
	}
}

func TestE2E_SpreadSkipsContacted(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "1m")
	env.backend.seedContact("Bob", "1m")

	// Log an interaction with Alice
	env.run(t, "log", "Alice", "--note", "coffee")

	stdout, _, err := env.run(t, "spread", "--apply")
	if err != nil {
		t.Fatalf("frm spread failed: %v", err)
	}
	// Only Bob should be spread (Alice was already contacted)
	if !strings.Contains(stdout, "Bob") {
		t.Errorf("expected Bob in spread output, got: %s", stdout)
	}
	if strings.Contains(stdout, "Alice") {
		t.Errorf("Alice should be skipped (already contacted), got: %s", stdout)
	}
}

func TestE2E_Add(t *testing.T) {
	env := setupTest(t)

	// Add a basic contact
	stdout, _, err := env.run(t, "add", "Jane Doe")
	if err != nil {
		t.Fatalf("frm add failed: %v", err)
	}
	if !strings.Contains(stdout, "Added contact Jane Doe") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Verify the contact exists via contacts command
	stdout, _, err = env.run(t, "contacts")
	if err != nil {
		t.Fatalf("frm contacts failed: %v", err)
	}
	if !strings.Contains(stdout, "Jane Doe") {
		t.Errorf("expected Jane Doe in contacts, got: %s", stdout)
	}

	// Add a contact with all optional fields
	stdout, _, err = env.run(t, "add", "Bob Smith",
		"--email", "bob@example.com",
		"--phone", "555-1234",
		"--org", "Acme Corp",
		"--url", "https://bob.example.com",
	)
	if err != nil {
		t.Fatalf("frm add with flags failed: %v", err)
	}
	if !strings.Contains(stdout, "Added contact Bob Smith") {
		t.Errorf("unexpected output: %s", stdout)
	}

	// Verify the contact card has the expected fields
	card := env.getContactCard("Bob Smith")
	if card == nil {
		t.Fatal("Bob Smith not found in backend")
	}
	if card.PreferredValue(vcard.FieldEmail) != "bob@example.com" {
		t.Errorf("expected email bob@example.com, got %q", card.PreferredValue(vcard.FieldEmail))
	}
	if card.PreferredValue(vcard.FieldTelephone) != "555-1234" {
		t.Errorf("expected phone 555-1234, got %q", card.PreferredValue(vcard.FieldTelephone))
	}
	if card.PreferredValue(vcard.FieldOrganization) != "Acme Corp" {
		t.Errorf("expected org Acme Corp, got %q", card.PreferredValue(vcard.FieldOrganization))
	}
	if card.PreferredValue(vcard.FieldURL) != "https://bob.example.com" {
		t.Errorf("expected url https://bob.example.com, got %q", card.PreferredValue(vcard.FieldURL))
	}
}

func TestE2E_ContextNoEmail(t *testing.T) {
	messages := map[string][]mockMessage{
		"alice@example.com": {
			{Subject: "Should not appear", ReceivedAt: "2024-01-15T10:00:00Z"},
		},
	}
	env := setupTestWithJMAP(t, messages)
	// Contact without email field
	env.backend.seedContact("Bob", "2w")

	stdout, stderr, err := env.run(t, "context", "Bob")
	if err != nil {
		t.Fatalf("frm context failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}
	if strings.Contains(stdout, "Should not appear") {
		t.Errorf("email context should not appear for contact without email, got: %s", stdout)
	}
	if strings.Contains(stdout, "Recent emails:") {
		t.Errorf("should not show email header for contact without email, got: %s", stdout)
	}
}
