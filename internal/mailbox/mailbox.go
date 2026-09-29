// Package mailbox sorts the messages of IMAP mailboxes: it selects them,
// reads them without marking them as read, classifies them and tags,
// copies or moves them.
package mailbox

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/bornholm/indecis-email-triage/internal/config"
	"github.com/bornholm/indecis-email-triage/internal/mailtext"
	"github.com/bornholm/indecis-email-triage/internal/state"
	"github.com/bornholm/indecis-email-triage/internal/triage"
)

// batch is the number of messages fetched and classified together.
const batch = 20

// Dial connects and logs in.
func Dial(c config.IMAP) (*imapclient.Client, error) {
	opts := &imapclient.Options{TLSConfig: &tls.Config{InsecureSkipVerify: c.InsecureSkipVerify}}
	var (
		client *imapclient.Client
		err    error
	)
	switch c.Security {
	case "starttls":
		client, err = imapclient.DialStartTLS(c.Address, opts)
	case "none":
		client, err = imapclient.DialInsecure(c.Address, opts)
	default:
		client, err = imapclient.DialTLS(c.Address, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", c.Address, err)
	}
	if err := client.Login(c.Username, c.Password).Wait(); err != nil {
		client.Close()
		return nil, fmt.Errorf("login as %s: %w", c.Username, err)
	}
	return client, nil
}

// Stats counts what a run did.
type Stats struct {
	Seen, Sorted, Uncertain, Skipped int
	ByCategory                       map[string]int
}

// Sorter runs the sort.
type Sorter struct {
	Cfg        *config.Config
	Classifier *triage.Classifier
	Log        *slog.Logger
	DryRun     bool // classify and report, change nothing
	Now        func() time.Time
}

// Run sorts every configured mailbox.
func (s *Sorter) Run(ctx context.Context, client *imapclient.Client) (Stats, error) {
	st, err := state.Load(s.Cfg.State)
	if err != nil {
		return Stats{}, fmt.Errorf("state %s: %w", s.Cfg.State, err)
	}
	stats := Stats{ByCategory: map[string]int{}}
	delim, err := delimiter(client)
	if err != nil {
		return stats, err
	}
	for _, name := range s.Cfg.Mailboxes {
		if err := s.mailbox(ctx, client, st, name, delim, &stats); err != nil {
			return stats, fmt.Errorf("%s: %w", name, err)
		}
		if !s.DryRun {
			if err := st.Save(); err != nil {
				return stats, err
			}
		}
	}
	return stats, nil
}

func delimiter(client *imapclient.Client) (string, error) {
	l, err := client.List("", "", nil).Collect()
	if err != nil {
		return "", fmt.Errorf("list: %w", err)
	}
	if len(l) > 0 && l[0].Delim != 0 {
		return string(l[0].Delim), nil
	}
	return "/", nil
}

func (s *Sorter) mailbox(ctx context.Context, client *imapclient.Client, st *state.State, name, delim string, stats *Stats) error {
	sel, err := client.Select(name, &imap.SelectOptions{ReadOnly: s.DryRun}).Wait()
	if err != nil {
		return err
	}
	if s.Cfg.Action.Mode == "keyword" && !s.DryRun && !slices.Contains(sel.PermanentFlags, imap.FlagWildcard) {
		return errors.New("the server does not accept new keywords here: use action.mode copy or move")
	}
	mb := st.Mailbox(name, sel.UIDValidity)

	criteria := &imap.SearchCriteria{}
	if s.Cfg.Select.Unseen {
		criteria.NotFlag = []imap.Flag{imap.FlagSeen}
	}
	since, _ := s.Cfg.Select.SinceTime(s.now())
	criteria.Since = since
	found, err := client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	var uids []imap.UID
	for _, u := range found.AllUIDs() {
		if !mb.IsDone(uint32(u)) {
			uids = append(uids, u)
		}
	}
	// Most recent first: UIDs grow with arrival.
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	if lim := s.Cfg.Select.Limit; lim > 0 && len(uids) > lim {
		uids = uids[:lim]
	}
	s.Log.Info("mailbox", "name", name, "to sort", len(uids))
	if len(uids) == 0 {
		return nil
	}
	if s.Cfg.Action.Mode != "keyword" && !s.DryRun {
		if err := s.ensureFolders(client, delim); err != nil {
			return err
		}
	}
	for start := 0; start < len(uids); start += batch {
		if err := ctx.Err(); err != nil {
			return err
		}
		part := uids[start:min(start+batch, len(uids))]
		if err := s.sortBatch(ctx, client, mb, part, delim, stats); err != nil {
			return err
		}
	}
	if s.Cfg.Action.Mode == "move" && !s.DryRun && !client.Caps().Has(imap.CapMove) {
		// Without MOVE, moved messages were copied and flagged \Deleted.
		if err := client.Expunge().Close(); err != nil {
			return fmt.Errorf("expunge: %w", err)
		}
	}
	return nil
}

func (s *Sorter) sortBatch(ctx context.Context, client *imapclient.Client, mb *state.Mailbox, uids []imap.UID, delim string, stats *Stats) error {
	section := &imap.FetchItemBodySection{Peek: true} // BODY.PEEK[]: not marked as read
	msgs, err := client.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{UID: true, BodySection: []*imap.FetchItemBodySection{section}}).Collect()
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	var emails []mailtext.Email
	var ids []imap.UID
	for _, m := range msgs {
		e, err := mailtext.Parse(strings.NewReader(string(m.FindBodySection(section))))
		if err != nil {
			s.Log.Warn("unreadable message", "uid", m.UID, "error", err)
			stats.Skipped++
			continue
		}
		emails = append(emails, e)
		ids = append(ids, m.UID)
	}
	texts := make([]string, len(emails))
	for i, e := range emails {
		texts[i] = e.Text
	}
	results, err := s.Classifier.Classify(ctx, texts...)
	if err != nil {
		return err
	}
	for i, r := range results {
		stats.Seen++
		uid, e := ids[i], emails[i]
		if r.Confidence < s.Cfg.Action.Threshold {
			stats.Uncertain++
			s.Log.Info("uncertain", "uid", uid, "subject", e.Subject, "best", r.Category.Name, "confidence", round(r.Confidence))
			if !s.DryRun {
				mb.MarkDone(uint32(uid))
			}
			continue
		}
		target := ""
		switch s.Cfg.Action.Mode {
		case "keyword":
			target = s.Cfg.Keyword(r.Category)
		default:
			target = s.folder(r.Category, delim)
		}
		s.Log.Info("sorted", "uid", uid, "subject", e.Subject, "from", e.From, "category", r.Category.Name,
			"confidence", round(r.Confidence), s.Cfg.Action.Mode, target, "dry_run", s.DryRun)
		stats.Sorted++
		stats.ByCategory[r.Category.Name]++
		if s.DryRun {
			continue
		}
		if err := s.act(client, uid, target); err != nil {
			return fmt.Errorf("uid %d: %w", uid, err)
		}
		mb.MarkDone(uint32(uid))
	}
	return nil
}

func (s *Sorter) act(client *imapclient.Client, uid imap.UID, target string) error {
	set := imap.UIDSetNum(uid)
	switch s.Cfg.Action.Mode {
	case "keyword":
		return client.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.Flag(target)}}, nil).Close()
	case "copy":
		_, err := client.Copy(set, target).Wait()
		return err
	default: // move
		if client.Caps().Has(imap.CapMove) {
			_, err := client.Move(set, target).Wait()
			return err
		}
		if _, err := client.Copy(set, target).Wait(); err != nil {
			return err
		}
		return client.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
	}
}

// folder is the folder of a category, written with the server's
// hierarchy delimiter.
func (s *Sorter) folder(cat config.Category, delim string) string {
	return strings.ReplaceAll(s.Cfg.FolderOf(cat), "/", delim)
}

func (s *Sorter) ensureFolders(client *imapclient.Client, delim string) error {
	l, err := client.List("", "*", nil).Collect()
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	exists := map[string]bool{}
	for _, d := range l {
		exists[d.Mailbox] = true
	}
	for _, cat := range s.Cfg.Categories {
		f := s.folder(cat, delim)
		// Create the parents too, for servers that do not.
		parts := strings.Split(f, delim)
		for i := range parts {
			p := strings.Join(parts[:i+1], delim)
			if exists[p] {
				continue
			}
			if err := client.Create(p, nil).Wait(); err != nil {
				return fmt.Errorf("create %s: %w", p, err)
			}
			exists[p] = true
			s.Log.Info("folder created", "name", p)
		}
	}
	return nil
}

func (s *Sorter) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func round(x float64) float64 { return float64(int(x*1000+0.5)) / 1000 }
