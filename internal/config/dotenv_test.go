package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnvLine(t *testing.T) {
	cases := []struct {
		line      string
		key       string
		value     string
		recognise bool
	}{
		{line: `ADDR=:8080`, key: "ADDR", value: ":8080", recognise: true},
		{line: `export ADDR=:9090`, key: "ADDR", value: ":9090", recognise: true},
		{line: `  SPACED  =  value  `, key: "SPACED", value: "value", recognise: true},
		{line: `QUOTED="a b # c"`, key: "QUOTED", value: "a b # c", recognise: true},
		{line: `RAW='keep # this'`, key: "RAW", value: "keep # this", recognise: true},
		{line: `TRAILING=value # note`, key: "TRAILING", value: "value", recognise: true},
		{line: `HASHINVALUE=pa#ss`, key: "HASHINVALUE", value: "pa#ss", recognise: true},
		{line: `ESCAPED="line\nbreak"`, key: "ESCAPED", value: "line\nbreak", recognise: true},
		{line: `URL=postgres://u:p@host:5432/db?sslmode=disable`, key: "URL",
			value: "postgres://u:p@host:5432/db?sslmode=disable", recognise: true},
		{line: `EMPTY=`, key: "EMPTY", value: "", recognise: true},
		{line: `# comment`, recognise: false},
		{line: ``, recognise: false},
		{line: `no equals sign`, recognise: false},
		{line: `1BAD=x`, recognise: false},
	}

	for _, c := range cases {
		key, value, ok := parseDotEnvLine(c.line)
		if ok != c.recognise {
			t.Errorf("%q: recognised = %v, want %v", c.line, ok, c.recognise)
			continue
		}
		if ok && (key != c.key || value != c.value) {
			t.Errorf("%q: got %q=%q, want %q=%q", c.line, key, value, c.key, c.value)
		}
	}
}

func TestLoadDotEnvDoesNotOverrideRealEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	contents := "FROM_FILE=file-value\nALREADY_SET=file-value\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ALREADY_SET", "real-value")
	t.Setenv("FROM_FILE", "")
	os.Unsetenv("FROM_FILE")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	defer os.Unsetenv("FROM_FILE")

	if got := os.Getenv("ALREADY_SET"); got != "real-value" {
		t.Errorf("the file overwrote a real environment variable: got %q", got)
	}
	if got := os.Getenv("FROM_FILE"); got != "file-value" {
		t.Errorf("FROM_FILE = %q, want file-value", got)
	}
}

func TestLoadDotEnvIgnoresAMissingFile(t *testing.T) {
	if err := LoadDotEnv(filepath.Join(t.TempDir(), "absent")); err != nil {
		t.Fatalf("a missing .env should not be an error, got %v", err)
	}
}

func TestClampSeconds(t *testing.T) {
	cfg := &Config{MinSpinSeconds: 3, MaxSpinSeconds: 300}
	for _, c := range []struct{ in, want int }{{0, 3}, {3, 3}, {15, 15}, {300, 300}, {9000, 300}} {
		if got := cfg.ClampSeconds(c.in); got != c.want {
			t.Errorf("ClampSeconds(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
