package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	got, err := ParseDotEnv(`# IMAP account
IMAP_ADDRESS=imap.example.org:993
export IMAP_USER = me@example.org
IMAP_PASSWORD="p#ss \"quoted\"\tand\\"
SINGLE='raw \n $HOME # kept'
COMMENT=value # not part of it
HASH=a#b
EMPTY=
`)
	want := [][2]string{
		{"IMAP_ADDRESS", "imap.example.org:993"},
		{"IMAP_USER", "me@example.org"},
		{"IMAP_PASSWORD", "p#ss \"quoted\"\tand\\"},
		{"SINGLE", `raw \n $HOME # kept`},
		{"COMMENT", "value"},
		{"HASH", "a#b"},
		{"EMPTY", ""},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("ParseDotEnv = %q, %v; want %q", got, err, want)
	}
	for _, in := range []string{"NOEQUAL", "1BAD=x", `A="open`, `A="x" y`} {
		if _, err := ParseDotEnv(in); err == nil {
			t.Errorf("ParseDotEnv(%q): no error", in)
		}
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := LoadDotEnv(filepath.Join(dir, ".env")); err != nil {
		t.Fatalf("missing file: %v", err)
	}
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("DOTENV_TEST_NEW=from file\nDOTENV_TEST_SET=from file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_SET", "from environment")
	t.Setenv("DOTENV_TEST_NEW", "")
	os.Unsetenv("DOTENV_TEST_NEW")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv("DOTENV_TEST_NEW"); v != "from file" {
		t.Errorf("DOTENV_TEST_NEW = %q", v)
	}
	if v := os.Getenv("DOTENV_TEST_SET"); v != "from environment" {
		t.Errorf("the environment should win: DOTENV_TEST_SET = %q", v)
	}
}
