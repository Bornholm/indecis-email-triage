package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadDotEnv sets the variables of a .env file, KEY=value per line, that
// are not already set: the environment wins. A missing file is not an
// error. Lines may start with "export"; # starts a comment, except inside
// quotes. A value in double quotes reads \n, \t, \" and \\; a value in
// single quotes is taken as is. Variables are not expanded in values.
func LoadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	vars, err := ParseDotEnv(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for _, kv := range vars {
		if _, set := os.LookupEnv(kv[0]); set {
			continue
		}
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return nil
}

// ParseDotEnv reads the content of a .env file into key, value pairs, in
// the order of the file.
func ParseDotEnv(text string) ([][2]string, error) {
	var vars [][2]string
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !envName.MatchString(key) {
			return nil, fmt.Errorf("line %d: expected KEY=value", i+1)
		}
		value, err := dotEnvValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("line %d: %s: %w", i+1, key, err)
		}
		vars = append(vars, [2]string{key, value})
	}
	return vars, nil
}

func dotEnvValue(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	switch q := v[0]; q {
	case '\'', '"':
		end := 1
		var b strings.Builder
		for ; end < len(v) && v[end] != q; end++ {
			c := v[end]
			if q == '"' && c == '\\' && end+1 < len(v) {
				end++
				switch c = v[end]; c {
				case 'n':
					c = '\n'
				case 't':
					c = '\t'
				case '"', '\\':
				default:
					b.WriteByte('\\')
				}
			}
			b.WriteByte(c)
		}
		if end == len(v) {
			return "", fmt.Errorf("missing closing %c", q)
		}
		if rest := strings.TrimSpace(v[end+1:]); rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("unexpected %q after the closing %c", rest, q)
		}
		return b.String(), nil
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v), nil
}
