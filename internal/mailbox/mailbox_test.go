package mailbox

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/bornholm/indecis-email-triage/internal/config"
	"github.com/bornholm/indecis-email-triage/internal/triage"
)

// server starts an in-memory IMAP server holding messages in INBOX.
func server(t *testing.T, messages []string) string {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("alice", "secret")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	c, err := imapclient.DialInsecure(ln.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Login("alice", "secret").Wait(); err != nil {
		t.Fatal(err)
	}
	for _, m := range messages {
		cmd := c.Append("INBOX", int64(len(m)), nil)
		cmd.Write([]byte(m))
		cmd.Close()
		if _, err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	return ln.Addr().String()
}

func email(subject, body string) string {
	return fmt.Sprintf("From: x@example.org\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n", subject, body)
}

var inbox = []string{
	email("Relance facture 2024-118", "Bonjour, la facture du 12 mars reste impayée. Merci de régler sous huitaine."),
	email("Invitation : point projet jeudi", "Je vous propose une réunion jeudi à 10 h en salle Mercure."),
	email("Votre facture de septembre", "Votre facture d'électricité est disponible dans votre espace client."),
	email("Réunion de lancement", "Rendez-vous lundi à 14 h pour le lancement du projet, visio en pièce jointe."),
}

var want = []string{"Facturation", "Réunion", "Facturation", "Réunion"}

func modelDir(t *testing.T) string {
	dir := os.Getenv("INDECIS_BEKKO_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache/indecis/models/bekko-embedding-v1-a8m")
	}
	if _, err := os.Stat(filepath.Join(dir, "model.safetensors")); err != nil {
		t.Skipf("model absent (%s): set INDECIS_BEKKO_DIR", dir)
	}
	return dir
}

func setup(t *testing.T, mode string) (*config.Config, *Sorter, string) {
	t.Helper()
	addr := server(t, inbox)
	cfg := &config.Config{
		IMAP:   config.IMAP{Address: addr, Security: "none", Username: "alice", Password: "secret"},
		Model:  modelDir(t),
		State:  filepath.Join(t.TempDir(), "state.json"),
		Action: config.Action{Mode: mode},
		Categories: []config.Category{
			{Name: "Facturation", Description: "Factures, paiements, relances", Examples: []string{"Votre facture n° 4521 est disponible", "Relance : paiement en retard"}},
			{Name: "Réunion", Description: "Invitations et organisation de réunions", Examples: []string{"Réunion de suivi mardi à 9 h", "Pouvez-vous décaler le point de lundi ?"}},
		},
	}
	path := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(path, []byte(fmt.Sprintf("imap: {address: %q, security: none, username: alice, password: secret}\nmodel: %q\naction: {mode: %s}\ncategories: [{name: Facturation}, {name: Réunion}]\n", addr, cfg.Model, mode)), 0o600)
	checked, err := config.Load(path) // applies the defaults
	if err != nil {
		t.Fatal(err)
	}
	cfg.Mailboxes, cfg.Action = checked.Mailboxes, checked.Action
	for i := range cfg.Categories {
		cfg.Categories[i].Keyword, cfg.Categories[i].Folder = checked.Categories[i].Keyword, checked.Categories[i].Folder
	}
	c, err := triage.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, &Sorter{Cfg: cfg, Classifier: c, Log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))}, addr
}

func connect(t *testing.T, cfg *config.Config) *imapclient.Client {
	t.Helper()
	c, err := Dial(cfg.IMAP)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func flags(t *testing.T, c *imapclient.Client, mailbox string) [][]imap.Flag {
	t.Helper()
	sel, err := c.Select(mailbox, nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if sel.NumMessages == 0 {
		return nil
	}
	msgs, err := c.Fetch(imap.SeqSetNum(seq(sel.NumMessages)...), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	out := make([][]imap.Flag, len(msgs))
	for i, m := range msgs {
		out[i] = m.Flags
	}
	return out
}

func seq(n uint32) []uint32 {
	s := make([]uint32, n)
	for i := range s {
		s[i] = uint32(i + 1)
	}
	return s
}

func TestKeywordMode(t *testing.T) {
	cfg, s, _ := setup(t, "keyword")
	ctx := context.Background()

	// Dry run: nothing changes.
	s.DryRun = true
	stats, err := s.Run(ctx, connect(t, cfg))
	if err != nil || stats.Sorted != 4 {
		t.Fatalf("dry run: %+v, %v", stats, err)
	}
	for _, f := range flags(t, connect(t, cfg), "INBOX") {
		if len(f) != 0 {
			t.Fatalf("dry run changed flags: %v", f)
		}
	}

	s.DryRun = false
	if stats, err = s.Run(ctx, connect(t, cfg)); err != nil || stats.Sorted != 4 {
		t.Fatalf("run: %+v, %v", stats, err)
	}
	c := connect(t, cfg)
	for i, f := range flags(t, c, "INBOX") {
		kw := imap.Flag(cfg.Keyword(category(cfg, want[i])))
		if !slices.Contains(f, kw) {
			t.Errorf("message %d: flags %v, want %s", i+1, f, kw)
		}
		if slices.Contains(f, imap.FlagSeen) {
			t.Errorf("message %d marked as read", i+1)
		}
	}

	// Sorted once: a second run finds nothing to do.
	if stats, err = s.Run(ctx, connect(t, cfg)); err != nil || stats.Seen != 0 {
		t.Fatalf("second run: %+v, %v", stats, err)
	}
}

func TestCopyMode(t *testing.T) {
	cfg, s, _ := setup(t, "copy")
	if _, err := s.Run(context.Background(), connect(t, cfg)); err != nil {
		t.Fatal(err)
	}
	c := connect(t, cfg)
	for _, name := range []string{"Facturation", "Réunion"} {
		folder := cfg.FolderOf(category(cfg, name))
		if n := len(flags(t, c, folder)); n != 2 {
			t.Errorf("%s: %d messages, want 2", folder, n)
		}
	}
	if n := len(flags(t, c, "INBOX")); n != 4 {
		t.Errorf("INBOX: %d messages after copy, want 4", n)
	}
}

func category(cfg *config.Config, name string) config.Category {
	for _, c := range cfg.Categories {
		if c.Name == name {
			return c
		}
	}
	panic(name)
}

func TestMoveMode(t *testing.T) {
	cfg, s, _ := setup(t, "move")
	if _, err := s.Run(context.Background(), connect(t, cfg)); err != nil {
		t.Fatal(err)
	}
	c := connect(t, cfg)
	if n := len(flags(t, c, "INBOX")); n != 0 {
		t.Errorf("INBOX: %d messages left after move", n)
	}
	for _, name := range []string{"Facturation", "Réunion"} {
		if n := len(flags(t, c, cfg.FolderOf(category(cfg, name)))); n != 2 {
			t.Errorf("%s: %d messages, want 2", name, n)
		}
	}
}
