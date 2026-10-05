// Package config reads indecis-mail's YAML configuration. Environment
// variables are expanded in the values (not in comments nor keys):
// ${VAR}, ${VAR:-default} (default when VAR is unset or empty),
// ${VAR:?message} (an error when it is), and $$ for a literal $.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the whole file.
type Config struct {
	IMAP IMAP `yaml:"imap"`
	// Model is an indecis model directory: a model fine-tuned for emails, or
	// a bare backbone such as bekko-embedding-v1-a8m.
	Model      string     `yaml:"model"`
	Mailboxes  []string   `yaml:"mailboxes"`
	Select     Select     `yaml:"select"`
	Action     Action     `yaml:"action"`
	Categories []Category `yaml:"categories"`
	// State is the file where processed messages are remembered, so that a
	// message is sorted once. Default: next to the configuration file.
	State string `yaml:"state"`
}

// IMAP describes the server and the account.
type IMAP struct {
	Address  string `yaml:"address"`  // host:port
	Security string `yaml:"security"` // tls (default), starttls or none
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// InsecureSkipVerify accepts any certificate, invalid ones included:
	// for a server with a self-signed certificate, or for tests.
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// Select chooses the messages to sort.
type Select struct {
	Unseen bool `yaml:"unseen"` // only unread messages
	// Since is a duration back from now ("7d", "36h") or a date
	// ("2026-09-01"); empty: no limit.
	Since string `yaml:"since"`
	Limit int    `yaml:"limit"` // most recent first; 0: no limit
}

// Action says what to do with a sorted message.
type Action struct {
	// Mode is keyword (an IMAP keyword on the message, it stays where it
	// is), copy (a copy in the category's folder) or move.
	Mode string `yaml:"mode"`
	// KeywordPrefix precedes the category's keyword (default "triage/").
	KeywordPrefix string `yaml:"keyword_prefix"`
	// Folder is the folder of a category for copy and move; {category} is
	// replaced by its folder name (default "Triage/{category}").
	Folder string `yaml:"folder"`
	// Threshold is the confidence under which nothing is done (0: always
	// act). The model's probabilities are not calibrated: tune it on a few
	// dozen of your emails with the classify command.
	Threshold float64 `yaml:"threshold"`
}

// Category is an option of the sort: a name, a description and a few
// example emails, which matter most.
type Category struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Examples    []string `yaml:"examples"`
	// Keyword and Folder override the names derived from Name.
	Keyword string `yaml:"keyword"`
	Folder  string `yaml:"folder"`
}

// Load reads, expands and checks a configuration file.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := expandNode(&doc, os.LookupEnv); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// Decode again from the expanded tree, refusing unknown fields: a
	// misspelled key must not be silently ignored.
	expanded, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(strings.NewReader(string(expanded)))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.State == "" {
		c.State = strings.TrimSuffix(path, ".yaml") + ".state.json"
	}
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// expandNode expands the variables of every scalar value of a YAML tree.
func expandNode(n *yaml.Node, lookup func(string) (string, bool)) error {
	if n.Kind == yaml.ScalarNode {
		v, err := Expand(n.Value, lookup)
		if err != nil {
			return err
		}
		if v != n.Value {
			// Tag and style reset: YAML infers the type of the new value
			// (a number for a threshold) and quotes it if needed.
			n.Value, n.Tag, n.Style = v, "", 0
		}
		return nil
	}
	for i, c := range n.Content {
		if n.Kind == yaml.MappingNode && i%2 == 0 {
			continue // keys
		}
		if err := expandNode(c, lookup); err != nil {
			return err
		}
	}
	return nil
}

var reference = regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:-|:\?)([^}]*))?\}`)

// Expand replaces the variable references of text, looking them up with
// lookup (os.LookupEnv in practice).
func Expand(text string, lookup func(string) (string, bool)) (string, error) {
	var errs []string
	out := reference.ReplaceAllStringFunc(text, func(m string) string {
		if m == "$$" {
			return "$"
		}
		g := reference.FindStringSubmatch(m)
		name, op, arg := g[1], g[2], g[3]
		v, ok := lookup(name)
		switch op {
		case ":-":
			if !ok || v == "" {
				return arg
			}
		case ":?":
			if !ok || v == "" {
				msg := arg
				if msg == "" {
					msg = "not set"
				}
				errs = append(errs, fmt.Sprintf("${%s}: %s", name, msg))
			}
		default:
			if !ok {
				errs = append(errs, fmt.Sprintf("${%s}: not set", name))
			}
		}
		return v
	})
	if len(errs) > 0 {
		return "", fmt.Errorf("environment: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

func (c *Config) check() error {
	if c.IMAP.Address == "" {
		return fmt.Errorf("imap.address missing")
	}
	switch c.IMAP.Security {
	case "":
		c.IMAP.Security = "tls"
	case "tls", "starttls", "none":
	default:
		return fmt.Errorf("imap.security: %q (expected tls, starttls or none)", c.IMAP.Security)
	}
	if c.Model == "" {
		return fmt.Errorf("model missing")
	}
	c.Model, c.State = home(c.Model), home(c.State)
	if len(c.Mailboxes) == 0 {
		c.Mailboxes = []string{"INBOX"}
	}
	if _, err := c.Select.SinceTime(time.Now()); err != nil {
		return err
	}
	switch c.Action.Mode {
	case "":
		c.Action.Mode = "keyword"
	case "keyword", "copy", "move":
	default:
		return fmt.Errorf("action.mode: %q (expected keyword, copy or move)", c.Action.Mode)
	}
	if c.Action.KeywordPrefix == "" {
		c.Action.KeywordPrefix = "triage/"
	}
	if c.Action.Folder == "" {
		c.Action.Folder = "Triage/{category}"
	}
	if c.Action.Threshold < 0 || c.Action.Threshold > 1 {
		return fmt.Errorf("action.threshold: %v out of [0, 1]", c.Action.Threshold)
	}
	if len(c.Categories) < 2 {
		return fmt.Errorf("at least two categories")
	}
	seen := map[string]bool{}
	for i := range c.Categories {
		cat := &c.Categories[i]
		if cat.Name == "" {
			return fmt.Errorf("categories[%d]: name missing", i)
		}
		if seen[cat.Name] {
			return fmt.Errorf("category %q defined twice", cat.Name)
		}
		seen[cat.Name] = true
		if cat.Keyword == "" {
			cat.Keyword = Atom(cat.Name)
		}
		if strings.ContainsAny(cat.Keyword, " ()[]{%*\"\\") {
			return fmt.Errorf("category %q: keyword %q is not an IMAP atom", cat.Name, cat.Keyword)
		}
		if cat.Folder == "" {
			cat.Folder = cat.Name
		}
	}
	return nil
}

// Keyword is the IMAP keyword of a category.
func (c *Config) Keyword(cat Category) string { return c.Action.KeywordPrefix + cat.Keyword }

// FolderOf is the folder of a category, for copy and move.
func (c *Config) FolderOf(cat Category) string {
	return strings.ReplaceAll(c.Action.Folder, "{category}", cat.Folder)
}

// SinceTime is the date from which messages are selected, zero for none.
func (s Select) SinceTime(now time.Time) (time.Time, error) {
	v := strings.TrimSpace(s.Since)
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, nil
	}
	if strings.HasSuffix(v, "d") {
		var days int
		if _, err := fmt.Sscanf(v, "%dd", &days); err == nil && days >= 0 {
			return now.AddDate(0, 0, -days), nil
		}
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return time.Time{}, fmt.Errorf("select.since: %q (expected 7d, 36h or 2026-09-01)", s.Since)
	}
	return now.Add(-d), nil
}

// Atom turns a category name into an IMAP keyword: lower case, ASCII
// letters, digits and dashes ("Facturation et paiement" gives
// "facturation-et-paiement").
func Atom(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(foldAccents(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

var accents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ô", "o", "ö", "o", "ó", "o", "ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ÿ", "y", "ñ", "n", "œ", "oe", "æ", "ae",
	"À", "a", "Â", "a", "Ç", "c", "É", "e", "È", "e", "Ê", "e", "Î", "i", "Ô", "o", "Ù", "u", "Û", "u",
)

func foldAccents(s string) string { return accents.Replace(s) }

// home expands a leading ~/ to the home directory.
func home(path string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, rest)
		}
	}
	return path
}
