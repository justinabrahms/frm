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

func (b *memBackend) seedContactFull(name, freq, email, phone, org string) {
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
	if phone != "" {
		card[vcard.FieldTelephone] = []*vcard.Field{{Value: phone}}
	}
	if org != "" {
		card[vcard.FieldOrganization] = []*vcard.Field{{Value: org}}
	}
	p := fmt.Sprintf("%s%s.vcf", abPath, strings.ReplaceAll(strings.ToLower(name), " ", "-"))
	b.contacts[p] = carddav.AddressObject{
		Path:    p,
		ModTime: time.Now(),
		ETag:    fmt.Sprintf("%d", time.Now().UnixNano()),
		Card:    card,
	}
}

func (b *memBackend) seedContactWithEmail(name, freq, email string) {
	b.seedContactFull(name, freq, email, "", "")
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

func TestE2E_Contacts_Deprecated(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Charlie", "1m")
	env.backend.seedContact("Alice", "")
	env.backend.seedContact("Bob", "2w")

	// "frm contacts" should behave like "frm list --all" and print a deprecation warning
	stdout, stderr, err := env.run(t, "contacts")
	if err != nil {
		t.Fatalf("frm contacts failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	if !strings.Contains(stderr, "deprecated") {
		t.Errorf("expected deprecation warning on stderr, got: %q", stderr)
	}

	// Should show all contacts in table format (header + 3 contacts)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (header + 3 contacts), got %d: %q", len(lines), stdout)
	}
	if !strings.Contains(lines[0], "NAME") || !strings.Contains(lines[0], "FREQ") {
		t.Errorf("expected table header, got: %s", lines[0])
	}
	if !strings.Contains(stdout, "Alice") || !strings.Contains(stdout, "Bob") || !strings.Contains(stdout, "Charlie") {
		t.Errorf("expected all contacts in output, got: %s", stdout)
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

func TestE2E_CheckJSONEnriched(t *testing.T) {
	env := setupTest(t)
	// Seed a contact with full details and a frequency
	env.backend.seedContactFull("Alice", "1w", "alice@example.com", "555-1234", "Acme Corp")
	// Set a group on Alice
	env.run(t, "group", "set", "Alice", "friends")

	// Log an overdue interaction (30 days ago) with a note
	overdueEntry := LogEntry{
		Contact: "Alice",
		Path:    abPath + "alice.vcf",
		Time:    time.Now().UTC().Add(-30 * 24 * time.Hour),
		Note:    "had coffee downtown",
	}
	data, _ := json.Marshal(overdueEntry)
	logPath := filepath.Join(env.configDir, "log.jsonl")
	if err := os.WriteFile(logPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("writing log: %v", err)
	}

	// Run check --json
	stdout, stderr, err := env.run(t, "check", "--json")
	if err != nil {
		t.Fatalf("frm check --json failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	var result []map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 overdue contact, got %d: %s", len(result), stdout)
	}

	c := result[0]
	if c["name"] != "Alice" {
		t.Errorf("expected name Alice, got %v", c["name"])
	}
	if c["email"] != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %v", c["email"])
	}
	if c["phone"] != "555-1234" {
		t.Errorf("expected phone 555-1234, got %v", c["phone"])
	}
	if c["org"] != "Acme Corp" {
		t.Errorf("expected org Acme Corp, got %v", c["org"])
	}
	if c["group"] != "friends" {
		t.Errorf("expected group friends, got %v", c["group"])
	}
	if c["last_note"] != "had coffee downtown" {
		t.Errorf("expected last_note 'had coffee downtown', got %v", c["last_note"])
	}
	if c["frequency"] != "1w" {
		t.Errorf("expected frequency 1w, got %v", c["frequency"])
	}
	if c["last_seen"] == nil || c["last_seen"] == "" {
		t.Error("expected last_seen to be set")
	}
	if c["ago"] == nil || c["ago"] == "" {
		t.Error("expected ago to be set")
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

func TestE2E_TriageCustomFrequency(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "")
	env.backend.seedContact("Bob", "")
	env.backend.seedContact("Charlie", "")

	// Alice gets custom 2w, Bob gets invalid then valid 3d, Charlie gets skip
	stdin := strings.NewReader("2w\nxyz\n3d\ns\n")
	stdout, stderr, err := env.runWithStdin(t, stdin, "triage")
	if err != nil {
		t.Fatalf("frm triage failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}

	// Verify summary includes custom count
	if !strings.Contains(stdout, "2 custom") {
		t.Errorf("expected 2 custom in summary, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 skipped") {
		t.Errorf("expected 1 skipped in summary, got: %s", stdout)
	}

	// Verify invalid input triggered an error message and re-prompt
	if !strings.Contains(stdout, "Invalid input") {
		t.Errorf("expected invalid input error message, got: %s", stdout)
	}

	// Verify Alice got 2w
	aliceCard := env.getContactCard("Alice")
	if aliceCard == nil {
		t.Fatal("Alice not found")
	}
	if freq := aliceCard.PreferredValue(fieldFrequency); freq != "2w" {
		t.Errorf("expected Alice frequency 2w, got %q", freq)
	}

	// Verify Bob got 3d (after invalid input was rejected)
	bobCard := env.getContactCard("Bob")
	if bobCard == nil {
		t.Fatal("Bob not found")
	}
	if freq := bobCard.PreferredValue(fieldFrequency); freq != "3d" {
		t.Errorf("expected Bob frequency 3d, got %q", freq)
	}

	// Verify Charlie has no frequency (skipped)
	charlieCard := env.getContactCard("Charlie")
	if charlieCard == nil {
		t.Fatal("Charlie not found")
	}
	if freq := charlieCard.PreferredValue(fieldFrequency); freq != "" {
		t.Errorf("expected Charlie no frequency, got %q", freq)
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
	env.backend.seedContact("Dave", "")

	// Ignore Dave: 2 tracked, 1 ignored, 1 untriaged out of 4
	env.run(t, "ignore", "Dave")

	env.run(t, "log", "Alice", "--note", "coffee")
	env.run(t, "log", "Alice", "--note", "lunch")
	env.run(t, "log", "Bob", "--note", "call")

	stdout, _, err := env.run(t, "stats")
	if err != nil {
		t.Fatalf("frm stats failed: %v", err)
	}
	if !strings.Contains(stdout, "Contacts:        4 total") {
		t.Errorf("expected 4 total contacts, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Tracked:       2") {
		t.Errorf("expected 2 tracked, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Ignored:       1") {
		t.Errorf("expected 1 ignored, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Untriaged:     1 (25%)") {
		t.Errorf("expected 1 untriaged (25%%), got: %s", stdout)
	}
	if !strings.Contains(stdout, "Most contacted:  Alice") {
		t.Errorf("expected Alice as most contacted, got: %s", stdout)
	}

	// JSON output includes untriaged and coverage_pct
	stdout, _, err = env.run(t, "stats", "--json")
	if err != nil {
		t.Fatalf("frm stats --json failed: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
	}
	if result["total_contacts"] != float64(4) {
		t.Errorf("expected total_contacts=4, got %v", result["total_contacts"])
	}
	if result["tracked"] != float64(2) {
		t.Errorf("expected tracked=2, got %v", result["tracked"])
	}
	if result["ignored"] != float64(1) {
		t.Errorf("expected ignored=1, got %v", result["ignored"])
	}
	if result["untriaged"] != float64(1) {
		t.Errorf("expected untriaged=1, got %v", result["untriaged"])
	}
	if result["coverage_pct"] != float64(75) {
		t.Errorf("expected coverage_pct=75, got %v", result["coverage_pct"])
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

	// List contacts in group via "group members"
	stdout, _, err = env.run(t, "group", "members", "friends")
	if err != nil {
		t.Fatalf("frm group members friends failed: %v", err)
	}
	if !strings.Contains(stdout, "Alice") {
		t.Errorf("expected Alice in friends group, got: %s", stdout)
	}
	if strings.Contains(stdout, "Bob") {
		t.Errorf("Bob should not be in friends group")
	}

	// Backwards compat: "group list <group>" still works but prints deprecation warning
	var stderr string
	stdout, stderr, err = env.run(t, "group", "list", "friends")
	if err != nil {
		t.Fatalf("frm group list friends (compat) failed: %v", err)
	}
	if !strings.Contains(stdout, "Alice") {
		t.Errorf("expected Alice in friends group via compat path, got: %s", stdout)
	}
	if !strings.Contains(stderr, "deprecated") {
		t.Errorf("expected deprecation warning on stderr, got: %q", stderr)
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

	// list --all --json (replaces old contacts --json)
	stdout, _, err := env.run(t, "list", "--all", "--json")
	if err != nil {
		t.Fatalf("frm list --all --json failed: %v", err)
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

	// Relative dates should be rejected
	_, stderr, err := env.run(t, "log", "Alice", "--note", "lunch", "--when", "-1w")
	if err == nil {
		t.Fatal("expected frm log --when with relative date to fail, but it succeeded")
	}
	if !strings.Contains(stderr, "invalid date") {
		t.Errorf("expected 'invalid date' in error, got: %s", stderr)
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

	// Verify the contact exists via list --all
	stdout, _, err = env.run(t, "list", "--all")
	if err != nil {
		t.Fatalf("frm list --all failed: %v", err)
	}
	if !strings.Contains(stdout, "Jane Doe") {
		t.Errorf("expected Jane Doe in list --all, got: %s", stdout)
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

func TestE2E_Edit(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContactWithEmail("Alice Smith", "2w", "alice@old.com")

	// Edit email and org
	stdout, stderr, err := env.run(t, "edit", "Alice Smith", "--email", "alice@new.com", "--org", "NewCorp")
	if err != nil {
		t.Fatalf("frm edit failed: %v\nstderr: %s\nstdout: %s", err, stderr, stdout)
	}
	if !strings.Contains(stdout, "Updated Alice Smith") {
		t.Errorf("expected 'Updated Alice Smith' in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "email=alice@new.com") {
		t.Errorf("expected email change in output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "org=NewCorp") {
		t.Errorf("expected org change in output, got: %s", stdout)
	}

	// Verify the card was actually updated
	card := env.getContactCard("Alice Smith")
	if card == nil {
		t.Fatal("Alice Smith not found in backend")
	}
	if card.PreferredValue(vcard.FieldEmail) != "alice@new.com" {
		t.Errorf("expected email alice@new.com, got %q", card.PreferredValue(vcard.FieldEmail))
	}
	if card.PreferredValue(vcard.FieldOrganization) != "NewCorp" {
		t.Errorf("expected org NewCorp, got %q", card.PreferredValue(vcard.FieldOrganization))
	}

	// Verify existing fields were not cleared (frequency should still be set)
	if freq := card.PreferredValue(fieldFrequency); freq != "2w" {
		t.Errorf("expected frequency 2w to be preserved, got %q", freq)
	}
}

func TestE2E_EditNoFlags(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Bob", "1m")

	// Edit with no flags should fail
	_, stderr, err := env.run(t, "edit", "Bob")
	if err == nil {
		t.Fatal("expected frm edit with no flags to fail")
	}
	if !strings.Contains(stderr, "no fields to update") {
		t.Errorf("expected 'no fields to update' error, got: %s", stderr)
	}
}

func TestE2E_EditNotFound(t *testing.T) {
	env := setupTest(t)

	// Edit a contact that doesn't exist
	_, stderr, err := env.run(t, "edit", "Nobody", "--email", "nobody@test.com")
	if err == nil {
		t.Fatal("expected frm edit for nonexistent contact to fail")
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("expected 'not found' error, got: %s", stderr)
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

func TestE2E_Init(t *testing.T) {
	backend := newMemBackend()
	handler := &carddav.Handler{Backend: backend}
	server := httptest.NewServer(handler)
	defer server.Close()

	configDir := t.TempDir()

	// Pipe stdin: choose carddav, custom provider, enter URL/username/password, decline JMAP
	input := fmt.Sprintf("c\n3\n%s/\ntestuser\ntestpass\nN\n", server.URL)
	stdin := strings.NewReader(input)

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "FRM_CONFIG_DIR="+configDir)
	cmd.Stdin = stdin
	var stdout2, stderr2 bytes.Buffer
	cmd.Stdout = &stdout2
	cmd.Stderr = &stderr2
	err := cmd.Run()
	if err != nil {
		t.Fatalf("frm init failed: %v\nstdout: %s\nstderr: %s", err, stdout2.String(), stderr2.String())
	}

	output := stdout2.String()
	if !strings.Contains(output, "Connection successful") {
		t.Errorf("expected connection success message, got: %s", output)
	}
	if !strings.Contains(output, "frm triage") {
		t.Errorf("expected next steps message, got: %s", output)
	}

	// Verify config file was written and is valid
	data, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	svc := cfg.Services[0]
	if svc.Type != "carddav" {
		t.Errorf("expected type carddav, got %q", svc.Type)
	}
	if svc.Username != "testuser" {
		t.Errorf("expected username testuser, got %q", svc.Username)
	}
	if svc.Password != "testpass" {
		t.Errorf("expected password testpass, got %q", svc.Password)
	}
	if !strings.Contains(svc.Endpoint, server.URL) {
		t.Errorf("expected endpoint containing %s, got %q", server.URL, svc.Endpoint)
	}
}

func TestE2E_InitAddService(t *testing.T) {
	backend := newMemBackend()
	handler := &carddav.Handler{Backend: backend}
	server := httptest.NewServer(handler)
	defer server.Close()

	configDir := t.TempDir()

	// Write initial config with one carddav service
	initialCfg := Config{
		Services: []ServiceConfig{{
			Type:     "carddav",
			Endpoint: server.URL + "/",
			Username: "existing",
			Password: "existing",
		}},
	}
	data, _ := json.Marshal(initialCfg)
	os.WriteFile(filepath.Join(configDir, "config.json"), data, 0o644)

	// Run init again, choose to add, add JMAP service
	input := "a\nj\nhttps://jmap.example.com/session\nmy-token\n"
	stdin := strings.NewReader(input)

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "FRM_CONFIG_DIR="+configDir)
	cmd.Stdin = stdin
	var stdout2, stderr2 bytes.Buffer
	cmd.Stdout = &stdout2
	cmd.Stderr = &stderr2
	err := cmd.Run()
	if err != nil {
		t.Fatalf("frm init (add) failed: %v\nstdout: %s\nstderr: %s", err, stdout2.String(), stderr2.String())
	}

	// Verify config now has both services
	data, err = os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsing config: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.Services[0].Type != "carddav" {
		t.Errorf("expected first service to be carddav, got %q", cfg.Services[0].Type)
	}
	if cfg.Services[1].Type != "jmap" {
		t.Errorf("expected second service to be jmap, got %q", cfg.Services[1].Type)
	}
	if cfg.Services[1].SessionEndpoint != "https://jmap.example.com/session" {
		t.Errorf("expected jmap endpoint, got %q", cfg.Services[1].SessionEndpoint)
	}
	if cfg.Services[1].Token != "my-token" {
		t.Errorf("expected jmap token, got %q", cfg.Services[1].Token)
	}
}

func TestE2E_InitWithPreset(t *testing.T) {
	configDir := t.TempDir()

	// Choose iCloud preset, enter credentials, validation fails, decline save
	input := "c\n1\napple-user\napple-pass\nN\n"
	stdin := strings.NewReader(input)

	cmd := exec.Command(binaryPath, "init")
	cmd.Env = append(os.Environ(), "FRM_CONFIG_DIR="+configDir)
	cmd.Stdin = stdin
	var stdout2, stderr2 bytes.Buffer
	cmd.Stdout = &stdout2
	cmd.Stderr = &stderr2
	err := cmd.Run()

	// Should fail because user declined to save after validation failure
	if err == nil {
		t.Fatal("expected frm init to fail when user declines to save, but it succeeded")
	}
	output := stdout2.String()
	if !strings.Contains(output, "iCloud") {
		t.Errorf("expected iCloud message, got: %s", output)
	}
}

func TestE2E_FuzzyMatch(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice Smith", "2w")
	env.backend.seedContact("Bob Jones", "1m")

	// Exact case-insensitive match should work as before.
	stdout, stderr, err := env.run(t, "context", "alice smith")
	if err != nil {
		t.Fatalf("exact case-insensitive lookup failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "Alice Smith") {
		t.Errorf("expected Alice Smith in output, got: %s", stdout)
	}
	if strings.Contains(stderr, "Using closest match") {
		t.Errorf("exact match should not produce fuzzy notice, stderr: %s", stderr)
	}

	// Fuzzy match with a typo (edit distance 1) should auto-select.
	stdout, stderr, err = env.run(t, "context", "Alic Smith")
	if err != nil {
		t.Fatalf("fuzzy lookup for 'Alic Smith' failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "Using closest match: Alice Smith") {
		t.Errorf("expected fuzzy match notice on stderr, got: %q", stderr)
	}
	if !strings.Contains(stdout, "Alice Smith") {
		t.Errorf("expected Alice Smith in output after fuzzy match, got: %s", stdout)
	}

	// Total mismatch should produce a not-found error.
	_, stderr, err = env.run(t, "context", "xyz")
	if err == nil {
		t.Fatal("expected error for 'xyz' lookup, but got success")
	}
	if !strings.Contains(stderr, `not found`) {
		t.Errorf("expected 'not found' in error, got: %s", stderr)
	}

	// Fuzzy match via findAllContactsMulti (used by track).
	stdout, stderr, err = env.run(t, "track", "Bbo Jones", "--every", "3w")
	if err != nil {
		t.Fatalf("fuzzy track for 'Bbo Jones' failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "Using closest match: Bob Jones") {
		t.Errorf("expected fuzzy match notice on stderr, got: %q", stderr)
	}
	if !strings.Contains(stdout, "Tracking Bob Jones every 3w") {
		t.Errorf("expected tracking confirmation, got: %s", stdout)
	}
}

func TestE2E_FuzzyMatchSuggestions(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")
	env.backend.seedContact("Alicx", "1m") // edit distance 1 from "Alicy"

	// When multiple contacts are within edit distance, show suggestions instead of auto-selecting.
	_, stderr, err := env.run(t, "context", "Alicy")
	if err == nil {
		t.Fatal("expected error when multiple fuzzy matches exist, but got success")
	}
	if !strings.Contains(stderr, "Did you mean") {
		t.Errorf("expected 'Did you mean' in error, got: %s", stderr)
	}
	if !strings.Contains(stderr, "Alice") {
		t.Errorf("expected 'Alice' in suggestions, got: %s", stderr)
	}
	if !strings.Contains(stderr, "Alicx") {
		t.Errorf("expected 'Alicx' in suggestions, got: %s", stderr)
	}
}

func TestE2E_TrackJSON(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "")

	stdout, _, err := env.run(t, "track", "Alice", "--every", "2w", "--json")
	if err != nil {
		t.Fatalf("frm track --json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
	}
	if result["action"] != "track" {
		t.Errorf("expected action=track, got %v", result["action"])
	}
	if result["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", result["name"])
	}
	if result["frequency"] != "2w" {
		t.Errorf("expected frequency=2w, got %v", result["frequency"])
	}
	if result["accounts"] != float64(1) {
		t.Errorf("expected accounts=1, got %v", result["accounts"])
	}
}

func TestE2E_IgnoreJSON(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")

	stdout, _, err := env.run(t, "ignore", "Alice", "--json")
	if err != nil {
		t.Fatalf("frm ignore --json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
	}
	if result["action"] != "ignore" {
		t.Errorf("expected action=ignore, got %v", result["action"])
	}
	if result["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", result["name"])
	}
	if result["accounts"] != float64(1) {
		t.Errorf("expected accounts=1, got %v", result["accounts"])
	}
}

func TestE2E_LogJSON(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice", "2w")

	stdout, _, err := env.run(t, "log", "Alice", "--note", "coffee chat", "--when", "2024-01-15", "--json")
	if err != nil {
		t.Fatalf("frm log --json failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
	}
	if result["action"] != "log" {
		t.Errorf("expected action=log, got %v", result["action"])
	}
	if result["contact"] != "Alice" {
		t.Errorf("expected contact=Alice, got %v", result["contact"])
	}
	if result["note"] != "coffee chat" {
		t.Errorf("expected note='coffee chat', got %v", result["note"])
	}
	// Time should be RFC3339 format
	timeStr, ok := result["time"].(string)
	if !ok || !strings.Contains(timeStr, "2024-01-15") {
		t.Errorf("expected time containing 2024-01-15, got %v", result["time"])
	}
}

func TestE2E_JSONError(t *testing.T) {
	env := setupTest(t)

	// Try to track a contact that doesn't exist, with --json
	stdout, _, err := env.run(t, "track", "Nobody", "--every", "2w", "--json")
	if err == nil {
		t.Fatal("expected frm track for nonexistent contact to fail")
	}

	var result map[string]string
	if jsonErr := json.Unmarshal([]byte(stdout), &result); jsonErr != nil {
		t.Fatalf("expected JSON error on stdout, got: %s", stdout)
	}
	if result["error"] == "" {
		t.Errorf("expected non-empty error field, got: %v", result)
	}
	if !strings.Contains(result["error"], "not found") {
		t.Errorf("expected 'not found' in error, got: %s", result["error"])
	}
}

func TestE2E_FuzzySubstringMatch(t *testing.T) {
	env := setupTest(t)
	env.backend.seedContact("Alice Smith", "2w")
	env.backend.seedContact("Bob Jones", "1m")

	// Substring match: "Alice" is contained in "Alice Smith".
	// With only one match, it should auto-select.
	stdout, stderr, err := env.run(t, "context", "Alice")
	if err != nil {
		t.Fatalf("substring lookup for 'Alice' failed: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stderr, "Using closest match: Alice Smith") {
		t.Errorf("expected fuzzy match notice on stderr, got: %q", stderr)
	}
	if !strings.Contains(stdout, "Alice Smith") {
		t.Errorf("expected Alice Smith in output, got: %s", stdout)
	}
}

func TestE2E_DryRun(t *testing.T) {
	t.Run("track", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "")

		stdout, _, err := env.run(t, "track", "Alice", "--every", "2w", "--dry-run")
		if err != nil {
			t.Fatalf("frm track --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would track Alice every 2w (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Verify the contact was NOT actually updated
		card := env.getContactCard("Alice")
		if card == nil {
			t.Fatal("Alice not found in backend")
		}
		freq := card.PreferredValue(fieldFrequency)
		if freq != "" {
			t.Errorf("expected empty frequency after dry run, got %q", freq)
		}
	})

	t.Run("track_json", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "")

		stdout, _, err := env.run(t, "track", "Alice", "--every", "2w", "--dry-run", "--json")
		if err != nil {
			t.Fatalf("frm track --dry-run --json failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
		}
		if result["dry_run"] != true {
			t.Errorf("expected dry_run=true, got %v", result["dry_run"])
		}
		if result["name"] != "Alice" {
			t.Errorf("expected name=Alice, got %v", result["name"])
		}

		// Verify not updated
		card := env.getContactCard("Alice")
		if freq := card.PreferredValue(fieldFrequency); freq != "" {
			t.Errorf("expected empty frequency after dry run, got %q", freq)
		}
	})

	t.Run("untrack", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "2w")

		stdout, _, err := env.run(t, "untrack", "Alice", "--dry-run")
		if err != nil {
			t.Fatalf("frm untrack --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would stop tracking Alice (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Frequency should still be set
		card := env.getContactCard("Alice")
		if freq := card.PreferredValue(fieldFrequency); freq != "2w" {
			t.Errorf("expected frequency 2w preserved after dry run, got %q", freq)
		}
	})

	t.Run("ignore", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "2w")

		stdout, _, err := env.run(t, "ignore", "Alice", "--dry-run")
		if err != nil {
			t.Fatalf("frm ignore --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would ignore Alice (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Should NOT be ignored
		card := env.getContactCard("Alice")
		if card.PreferredValue(fieldIgnore) != "" {
			t.Error("expected Alice NOT to be ignored after dry run")
		}
	})

	t.Run("unignore", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "")

		// First actually ignore Alice
		_, _, err := env.run(t, "ignore", "Alice")
		if err != nil {
			t.Fatalf("frm ignore failed: %v", err)
		}

		stdout, _, err := env.run(t, "unignore", "Alice", "--dry-run")
		if err != nil {
			t.Fatalf("frm unignore --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would unignore Alice (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Should still be ignored
		card := env.getContactCard("Alice")
		if card.PreferredValue(fieldIgnore) != "true" {
			t.Error("expected Alice to still be ignored after dry run")
		}
	})

	t.Run("log", func(t *testing.T) {
		env := setupTest(t)

		stdout, _, err := env.run(t, "log", "Alice", "--note", "coffee", "--dry-run")
		if err != nil {
			t.Fatalf("frm log --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would log interaction with Alice (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Log file should not exist
		logPath := filepath.Join(env.configDir, "log.jsonl")
		if _, err := os.Stat(logPath); err == nil {
			t.Error("expected no log file after dry run, but it exists")
		}
	})

	t.Run("snooze", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "2w")

		stdout, _, err := env.run(t, "snooze", "Alice", "--until", "2099-12-31", "--dry-run")
		if err != nil {
			t.Fatalf("frm snooze --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would snooze Alice until 2099-12-31 (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		card := env.getContactCard("Alice")
		if card.PreferredValue(fieldSnoozeUntil) != "" {
			t.Error("expected no snooze after dry run")
		}
	})

	t.Run("unsnooze", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "2w")

		// Actually snooze first
		_, _, err := env.run(t, "snooze", "Alice", "--until", "2099-12-31")
		if err != nil {
			t.Fatalf("frm snooze failed: %v", err)
		}

		stdout, _, err := env.run(t, "unsnooze", "Alice", "--dry-run")
		if err != nil {
			t.Fatalf("frm unsnooze --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would unsnooze Alice (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Should still be snoozed
		card := env.getContactCard("Alice")
		if card.PreferredValue(fieldSnoozeUntil) == "" {
			t.Error("expected Alice to still be snoozed after dry run")
		}
	})

	t.Run("add", func(t *testing.T) {
		env := setupTest(t)

		stdout, _, err := env.run(t, "add", "Jane Doe", "--dry-run")
		if err != nil {
			t.Fatalf("frm add --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would add contact Jane Doe (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Contact should NOT exist
		stdout, _, err = env.run(t, "list", "--all")
		if err != nil {
			t.Fatalf("frm list --all failed: %v", err)
		}
		if strings.Contains(stdout, "Jane Doe") {
			t.Error("expected Jane Doe NOT in list after dry run add")
		}
	})

	t.Run("edit", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContactWithEmail("Alice", "2w", "alice@old.com")

		stdout, _, err := env.run(t, "edit", "Alice", "--email", "alice@new.com", "--dry-run")
		if err != nil {
			t.Fatalf("frm edit --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would update Alice") {
			t.Errorf("unexpected output: %q", stdout)
		}
		if !strings.Contains(stdout, "dry run") {
			t.Errorf("expected dry run in output: %q", stdout)
		}

		// Email should NOT have changed
		card := env.getContactCard("Alice")
		if card.PreferredValue(vcard.FieldEmail) != "alice@old.com" {
			t.Errorf("expected email unchanged after dry run, got %q", card.PreferredValue(vcard.FieldEmail))
		}
	})

	t.Run("group_set", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "2w")

		stdout, _, err := env.run(t, "group", "set", "Alice", "friends", "--dry-run")
		if err != nil {
			t.Fatalf("frm group set --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would set Alice group to friends (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		card := env.getContactCard("Alice")
		if card.PreferredValue(fieldGroup) != "" {
			t.Error("expected no group after dry run")
		}
	})

	t.Run("group_unset", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "2w")

		// Actually set group first
		_, _, err := env.run(t, "group", "set", "Alice", "friends")
		if err != nil {
			t.Fatalf("frm group set failed: %v", err)
		}

		stdout, _, err := env.run(t, "group", "unset", "Alice", "--dry-run")
		if err != nil {
			t.Fatalf("frm group unset --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Would remove group from Alice (dry run)") {
			t.Errorf("unexpected output: %q", stdout)
		}

		// Group should still be set
		card := env.getContactCard("Alice")
		if card.PreferredValue(fieldGroup) != "friends" {
			t.Errorf("expected group friends preserved after dry run, got %q", card.PreferredValue(fieldGroup))
		}
	})

	t.Run("spread", func(t *testing.T) {
		env := setupTest(t)
		env.backend.seedContact("Alice", "1m")

		// spread --apply --dry-run should behave as dry run
		stdout, _, err := env.run(t, "spread", "--apply", "--dry-run")
		if err != nil {
			t.Fatalf("frm spread --apply --dry-run failed: %v", err)
		}
		if !strings.Contains(stdout, "Dry run") {
			t.Errorf("expected dry run message, got: %s", stdout)
		}

		card := env.getContactCard("Alice")
		if card.PreferredValue(fieldSnoozeUntil) != "" {
			t.Error("expected no snooze after spread --dry-run")
		}
	})
}
