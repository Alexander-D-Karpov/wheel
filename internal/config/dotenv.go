package config

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// LoadDotEnv reads a KEY=VALUE file and puts every pair into the process
// environment unless that variable is already set. A missing file is not an
// error: on a server the values usually come from systemd's EnvironmentFile.
//
// Supported syntax:
//
//	# comment
//	KEY=value            trailing " # comment" is stripped
//	export KEY=value
//	KEY="quoted value"   \n \r \t \" \\ are unescaped
//	KEY='raw value'      no escaping, no comment stripping
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		key, value, ok := parseDotEnvLine(sc.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return sc.Err()
}

func parseDotEnvLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")

	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:eq])
	if !validEnvKey(key) {
		return "", "", false
	}

	rest := strings.TrimSpace(line[eq+1:])
	switch {
	case strings.HasPrefix(rest, `"`):
		return key, unescapeDouble(cutQuoted(rest, '"')), true
	case strings.HasPrefix(rest, `'`):
		return key, cutQuoted(rest, '\''), true
	default:
		if i := strings.Index(rest, " #"); i >= 0 {
			rest = rest[:i]
		}
		return key, strings.TrimSpace(rest), true
	}
}

// cutQuoted returns the content between the leading quote and the next
// unescaped matching quote, or the remainder of the line if it is unterminated.
func cutQuoted(s string, quote byte) string {
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' && quote == '"' {
			i++
			continue
		}
		if s[i] == quote {
			return s[1:i]
		}
	}
	return s[1:]
}

func unescapeDouble(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i == len(s)-1 {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func validEnvKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c == '_':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
