package mailtext

import (
	"strings"
	"testing"
)

func TestPlainWithQuoteAndSignature(t *testing.T) {
	raw := "From: Alice <alice@example.org>\r\nSubject: =?UTF-8?Q?Relance_facture_n=C2=B0_2024-118?=\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" +
		"Bonjour,\r\n\r\nLa facture du 12 mars reste impayée.\r\n\r\n\r\nMerci.\r\n-- \r\nAlice, service comptable\r\n\r\nLe 3 mars, Bob a écrit :\r\n> ancienne demande\r\n"
	e, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	want := "Relance facture n° 2024-118\n\nBonjour,\n\nLa facture du 12 mars reste impayée.\n\nMerci."
	if e.Text != want || e.From != "alice@example.org" {
		t.Errorf("got %q from %q", e.Text, e.From)
	}
}

func TestReplyQuoteCut(t *testing.T) {
	got := Compose("Re: réunion", "D'accord pour jeudi.\n\nLe lun. 3 mars 2026 à 10:00, Bob <bob@x.fr> a écrit :\n> On se voit quand ?")
	if got != "Re: réunion\n\nD'accord pour jeudi." {
		t.Errorf("%q", got)
	}
}

func TestHTMLAndLatin1(t *testing.T) {
	raw := "Subject: Newsletter\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/html; charset=iso-8859-1\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"<html><head><style>p{}</style></head><body><p>Nos offres d'=E9t=E9&nbsp;!</p><p>Jusqu'=E0 -30&#160;%</p></body></html>\r\n--b--\r\n"
	e, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if e.Text != "Newsletter\n\nNos offres d'été !\nJusqu'à -30 %" {
		t.Errorf("%q", e.Text)
	}
}

func TestAttachmentIgnored(t *testing.T) {
	raw := "Subject: Contrat\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=m\r\n\r\n" +
		"--m\r\nContent-Type: text/plain\r\n\r\nVeuillez trouver le contrat signé.\r\n" +
		"--m\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=contrat.pdf\r\n\r\n%PDF-1.4 binary\r\n--m--\r\n"
	e, err := Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if e.Text != "Contrat\n\nVeuillez trouver le contrat signé." {
		t.Errorf("%q", e.Text)
	}
}

func TestLengthBound(t *testing.T) {
	got := Compose("Sujet", strings.Repeat("mot ", 1000))
	if len(got) > maxChars || !strings.HasPrefix(got, "Sujet\n\nmot mot") {
		t.Errorf("len %d", len(got))
	}
}
