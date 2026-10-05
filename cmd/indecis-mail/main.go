// Command indecis-mail sorts the emails of an IMAP mailbox among categories
// described in a YAML file, with a small indecis model running locally:
// no email leaves the machine.
//
//	indecis-mail check    -config mail.yaml   # connection, folders, keywords
//	indecis-mail classify -config mail.yaml message.eml ...
//	indecis-mail run      -config mail.yaml [-dry-run] [-watch 5m]
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/bornholm/indecis-email-triage/internal/config"
	"github.com/bornholm/indecis-email-triage/internal/mailbox"
	"github.com/bornholm/indecis-email-triage/internal/mailtext"
	"github.com/bornholm/indecis-email-triage/internal/triage"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: indecis-mail check|classify|run -config mail.yaml [options]")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	path := fs.String("config", "mail.yaml", "configuration file")
	dryRun := fs.Bool("dry-run", false, "run: classify and report, change nothing")
	watch := fs.Duration("watch", 0, "run: start again after this delay, until interrupted (0: once)")
	fs.Parse(os.Args[2:])
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := config.Load(*path)
	if err != nil {
		fail(err)
	}
	switch cmd {
	case "check":
		err = check(cfg)
	case "classify":
		err = classify(ctx, cfg, fs.Args())
	case "run":
		err = run(ctx, cfg, log, *dryRun, *watch)
	default:
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "indecis-mail:", err)
	os.Exit(1)
}

func check(cfg *config.Config) error {
	client, err := mailbox.Dial(cfg.IMAP)
	if err != nil {
		return err
	}
	defer client.Close()
	caps := client.Caps()
	fmt.Printf("connected to %s as %s\n", cfg.IMAP.Address, cfg.IMAP.Username)
	fmt.Printf("MOVE: %v, UIDPLUS: %v\n", caps.Has(imap.CapMove), caps.Has(imap.CapUIDPlus))
	list, err := client.List("", "*", nil).Collect()
	if err != nil {
		return err
	}
	var names []string
	for _, d := range list {
		names = append(names, d.Mailbox)
	}
	sort.Strings(names)
	fmt.Printf("folders: %s\n", strings.Join(names, ", "))
	for _, name := range cfg.Mailboxes {
		// Read-write: EXAMINE always answers empty PERMANENTFLAGS, which
		// would say that no keyword is accepted. Nothing is changed.
		sel, err := client.Select(name, nil).Wait()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		fmt.Printf("%s: %d messages, new keywords accepted: %v\n", name, sel.NumMessages, slices.Contains(sel.PermanentFlags, imap.FlagWildcard))
	}
	return client.Logout().Wait()
}

// classify sorts local .eml files, or standard input: to tune the
// categories and the threshold without touching the mailbox.
func classify(ctx context.Context, cfg *config.Config, files []string) error {
	c, err := triage.New(ctx, cfg)
	if err != nil {
		return err
	}
	var emails []mailtext.Email
	var names []string
	if len(files) == 0 {
		e, err := mailtext.Parse(os.Stdin)
		if err != nil {
			return err
		}
		emails, names = append(emails, e), append(names, "stdin")
	}
	for _, f := range files {
		r, err := os.Open(f)
		if err != nil {
			return err
		}
		e, err := mailtext.Parse(r)
		r.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		emails, names = append(emails, e), append(names, f)
	}
	texts := make([]string, len(emails))
	for i, e := range emails {
		texts[i] = e.Text
	}
	results, err := c.Classify(ctx, texts...)
	if err != nil {
		return err
	}
	for i, r := range results {
		mark := ""
		if r.Confidence < cfg.Action.Threshold {
			mark = " (under the threshold: left as is)"
		}
		fmt.Printf("%s\t%s\t%.0f%%%s\t%s\n", names[i], r.Category.Name, 100*r.Confidence, mark, emails[i].Subject)
	}
	return nil
}

func run(ctx context.Context, cfg *config.Config, log *slog.Logger, dryRun bool, watch time.Duration) error {
	start := time.Now()
	c, err := triage.New(ctx, cfg)
	if err != nil {
		return err
	}
	log.Info("model ready", "dir", cfg.Model, "categories", len(cfg.Categories), "duration", time.Since(start).Round(time.Millisecond))
	s := &mailbox.Sorter{Cfg: cfg, Classifier: c, Log: log, DryRun: dryRun}
	for {
		if err := once(ctx, cfg, s, log); err != nil {
			if watch == 0 {
				return err
			}
			log.Error("run", "error", err) // a watch survives a network error
		}
		if watch == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(watch):
		}
	}
}

func once(ctx context.Context, cfg *config.Config, s *mailbox.Sorter, log *slog.Logger) error {
	start := time.Now()
	client, err := mailbox.Dial(cfg.IMAP)
	if err != nil {
		return err
	}
	defer client.Close()
	stats, err := s.Run(ctx, client)
	if err != nil {
		return err
	}
	log.Info("done", "sorted", stats.Sorted, "uncertain", stats.Uncertain, "skipped", stats.Skipped,
		"by category", stats.ByCategory, "duration", time.Since(start).Round(time.Millisecond))
	return client.Logout().Wait()
}
