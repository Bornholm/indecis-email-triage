// Package mailtext extracts what a model should read from an email: the
// subject and the start of the body, as plain text, without the quoted
// previous messages nor the signature.
package mailtext

import (
	"bufio"
	"bytes"
	"html"
	"io"
	"mime"
	"regexp"
	"strings"

	_ "github.com/emersion/go-message/charset" // charsets beyond UTF-8
	"github.com/emersion/go-message/mail"
)

// maxChars bounds the text kept: the model reads 256 tokens, about 1,000
// characters of French prose.
const maxChars = 1500

// Email is what the model reads of a message, and a few fields for logs.
type Email struct {
	Subject string
	From    string
	Text    string // "subject\n\nbody", ready for the model
}

// Parse reads a message in RFC 5322 format.
func Parse(r io.Reader) (Email, error) {
	mr, err := mail.CreateReader(r)
	if err != nil {
		return Email{}, err
	}
	defer mr.Close()
	var e Email
	e.Subject, _ = mr.Header.Subject()
	if from, err := mr.Header.AddressList("From"); err == nil && len(from) > 0 {
		e.From = from[0].Address
	}
	var plain, htmlBody string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A malformed part: keep what was read so far.
			break
		}
		h, ok := p.Header.(*mail.InlineHeader)
		if !ok {
			continue // attachment
		}
		ct, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
		b, _ := io.ReadAll(io.LimitReader(p.Body, 256<<10))
		switch {
		case (ct == "" || ct == "text/plain") && plain == "":
			plain = string(b)
		case ct == "text/html" && htmlBody == "":
			htmlBody = string(b)
		}
	}
	body := plain
	if strings.TrimSpace(body) == "" {
		body = HTMLToText(htmlBody)
	}
	e.Text = Compose(e.Subject, body)
	return e, nil
}

// Compose builds the model's text from a subject and a body: quotes and
// signature removed, blank runs collapsed, bounded length.
func Compose(subject, body string) string {
	body = cleanBody(body)
	text := strings.TrimSpace(subject)
	if body != "" {
		if text != "" {
			text += "\n\n"
		}
		text += body
	}
	if len(text) > maxChars {
		cut := strings.LastIndex(text[:maxChars], " ")
		if cut < maxChars/2 {
			cut = maxChars
		}
		text = text[:cut]
	}
	return text
}

// Lines that introduce a quoted previous message, in French and English.
var quoteIntro = regexp.MustCompile(`(?i)^(le .{3,80} a écrit ?:|on .{3,80} wrote:|-{2,} ?(original message|message d'origine|forwarded message|message transféré) ?-{2,}|de ?: .+|from: .+)$`)

func cleanBody(body string) string {
	var out []string
	sc := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(body, "\r\n", "\n")))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	blank := 0
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t")
		trimmed := strings.TrimSpace(line)
		if line == "--" || line == "-- " || quoteIntro.MatchString(trimmed) {
			break // signature or quoted history: the rest is not this message
		}
		if strings.HasPrefix(trimmed, ">") {
			continue
		}
		if trimmed == "" {
			blank++
			continue
		}
		if blank > 0 && len(out) > 0 {
			out = append(out, "")
		}
		blank = 0
		out = append(out, trimmed)
	}
	return strings.Join(out, "\n")
}

var (
	dropBlocks = regexp.MustCompile(`(?is)<(script|style|head)\b.*?</(script|style|head)>`)
	breaks     = regexp.MustCompile(`(?i)<(br|/p|/div|/tr|/li|/h[1-6])\b[^>]*>`)
	tags       = regexp.MustCompile(`(?s)<[^>]*>`)
	spaces     = regexp.MustCompile(`[ \t\x{a0}]+`)
)

// HTMLToText keeps the text of an HTML body, one line per block.
func HTMLToText(s string) string {
	s = dropBlocks.ReplaceAllString(s, "")
	s = breaks.ReplaceAllString(s, "\n")
	s = tags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	var b bytes.Buffer
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(spaces.ReplaceAllString(line, " "))
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
