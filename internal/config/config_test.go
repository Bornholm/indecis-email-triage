package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := vars[k]; return v, ok }
}

func TestExpand(t *testing.T) {
	lookup := env(map[string]string{"USER": "alice", "EMPTY": ""})
	for in, want := range map[string]string{
		"u: ${USER}":            "u: alice",
		"p: ${PORT:-993}":       "p: 993",
		"e: ${EMPTY:-fallback}": "e: fallback",
		"s: ${USER:?required}":  "s: alice",
		"price: $$5 and $HOME":  "price: $5 and $HOME",
		"two: ${USER}-${USER}":  "two: alice-alice",
	} {
		got, err := Expand(in, lookup)
		if err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"${MISSING}", "${EMPTY:?password required}", "${MISSING:?}"} {
		if _, err := Expand(in, lookup); err == nil {
			t.Errorf("Expand(%q): no error", in)
		}
	}
	_, err := Expand("${A} ${B:?set B}", lookup)
	if err == nil || !strings.Contains(err.Error(), "${A}") || !strings.Contains(err.Error(), "set B") {
		t.Errorf("all missing variables reported: %v", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mail.yaml")
	os.WriteFile(path, []byte(`
imap:
  address: imap.example.org:993
  username: ${TEST_IMAP_USER}
  password: ${TEST_IMAP_PASSWORD:?set TEST_IMAP_PASSWORD}
model: /models/bekko
select:
  since: 7d
categories:
  - name: Facturation et paiement
    description: Factures, paiements, relances
    examples: ["Votre facture n° 4521 est disponible"]
  - name: Réunion
`), 0o600)
	t.Setenv("TEST_IMAP_USER", "alice")
	t.Setenv("TEST_IMAP_PASSWORD", "secret")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.IMAP.Username != "alice" || c.IMAP.Password != "secret" || c.IMAP.Security != "tls" {
		t.Errorf("imap: %+v", c.IMAP)
	}
	if c.Action.Mode != "keyword" || c.Mailboxes[0] != "INBOX" || c.State != filepath.Join(dir, "mail.state.json") {
		t.Errorf("defaults: %+v %v %s", c.Action, c.Mailboxes, c.State)
	}
	if k := c.Keyword(c.Categories[0]); k != "triage/facturation-et-paiement" {
		t.Errorf("keyword %q", k)
	}
	if k := c.Keyword(c.Categories[1]); k != "triage/reunion" {
		t.Errorf("keyword %q", k)
	}
	if f := c.FolderOf(c.Categories[1]); f != "Triage/Réunion" {
		t.Errorf("folder %q", f)
	}
	since, _ := c.Select.SinceTime(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC))
	if !since.Equal(time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("since %v", since)
	}

	t.Setenv("TEST_IMAP_PASSWORD", "")
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "set TEST_IMAP_PASSWORD") {
		t.Errorf("missing password: %v", err)
	}
}

func TestUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(path, []byte("imap: {address: x:993}\nmodel: m\ncategorie: []\n"), 0o600)
	if _, err := Load(path); err == nil {
		t.Error("a misspelled field is accepted")
	}
}

// The example configuration of the repository loads as is.
func TestExample(t *testing.T) {
	t.Setenv("IMAP_USER", "alice")
	t.Setenv("IMAP_PASSWORD", "secret")
	c, err := Load("../../config.example.yaml")
	if err != nil {
		t.Fatal(err)
	}
	h, _ := os.UserHomeDir()
	if !strings.HasPrefix(c.Model, h) || len(c.Categories) < 5 {
		t.Errorf("model %q, %d categories", c.Model, len(c.Categories))
	}
}

// Expanded values keep their type, and special characters survive.
func TestExpandedTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(path, []byte(`
imap:
  address: x:993
  password: ${TEST_PW}
model: m
select:
  limit: ${TEST_LIMIT:-50}
action:
  threshold: ${TEST_THRESHOLD:-0.35}
categories: [{name: A}, {name: B}]
`), 0o600)
	t.Setenv("TEST_PW", "p@ss: #word {x}")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Action.Threshold != 0.35 || c.Select.Limit != 50 || c.IMAP.Password != "p@ss: #word {x}" {
		t.Errorf("threshold %v, limit %d, password %q", c.Action.Threshold, c.Select.Limit, c.IMAP.Password)
	}
}
