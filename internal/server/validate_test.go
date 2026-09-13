package server

import "testing"

func TestCleanLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  Alice  ", "Alice"},
		{"two\nlines", "two lines"},
		{"tabs\tand   spaces", "tabs and spaces"},
		{"nul\x00byte", "nulbyte"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := cleanLabel(c.in); got != c.want {
			t.Errorf("cleanLabel(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	long := make([]rune, 300)
	for i := range long {
		long[i] = 'a'
	}
	if got := []rune(cleanLabel(string(long))); len(got) != maxLabelRunes {
		t.Errorf("a long label was cut to %d runes, want %d", len(got), maxLabelRunes)
	}
}

func TestCleanTitleAlwaysHasAName(t *testing.T) {
	if got := cleanTitle("   "); got == "" {
		t.Error("an empty title must fall back to a placeholder")
	}
	if got := cleanTitle("Friday draw"); got != "Friday draw" {
		t.Errorf("cleanTitle changed a good title to %q", got)
	}
}

func TestClampWeight(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{
		{1, 1}, {0, minWeight}, {-5, minWeight}, {0.001, minWeight}, {1e9, maxWeight},
	} {
		if got := clampWeight(c.in); got != c.want {
			t.Errorf("clampWeight(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseBulk(t *testing.T) {
	entries := parseBulk("Alice\n\nBob | 3\nCharlie|0,5\n  \nName | with | 2\nNo weight | here\n")

	want := []struct {
		label  string
		weight float64
	}{
		{"Alice", 1},
		{"Bob", 3},
		{"Charlie", 0.5},
		{"Name | with", 2},
		{"No weight | here", 1},
	}
	if len(entries) != len(want) {
		t.Fatalf("parsed %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i].Label != w.label || entries[i].Weight != w.weight {
			t.Errorf("entry %d = %q x%v, want %q x%v", i, entries[i].Label, entries[i].Weight, w.label, w.weight)
		}
	}
}

func TestCleanMode(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"exclusion", "exclusion"},
		{"selection", "selection"},
		{"", "selection"},
		{"nonsense", "selection"},
	} {
		if got := cleanMode(c.in); got != c.want {
			t.Errorf("cleanMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
