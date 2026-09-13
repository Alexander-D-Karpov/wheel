package server

import (
	"math"
	"strconv"
	"strings"
	"unicode"

	"github.com/Alexander-D-Karpov/wheel/internal/store"
)

const (
	maxLabelRunes = 120
	maxTitleRunes = 120
	minWeight     = 0.01
	maxWeight     = 100_000
)

// cleanLabel trims an entry label to one printable line of bounded length.
func cleanLabel(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	return truncate(s, maxLabelRunes)
}

// cleanTitle is cleanLabel with a fallback, since a wheel always shows a name.
func cleanTitle(s string) string {
	if t := truncate(cleanLabel(s), maxTitleRunes); t != "" {
		return t
	}
	return "Untitled wheel"
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return strings.TrimSpace(string(runes[:limit]))
}

// clampWeight keeps a weight positive and within a range the wheel can still
// draw sensibly.
func clampWeight(v float64) float64 {
	if math.IsNaN(v) || v < minWeight {
		return minWeight
	}
	if math.IsInf(v, 1) || v > maxWeight {
		return maxWeight
	}
	return v
}

func cleanMode(m string) string {
	if strings.TrimSpace(m) == store.ModeExclusion {
		return store.ModeExclusion
	}
	return store.ModeSelection
}

// parseWeightText reads a weight written by hand, accepting a comma decimal
// separator as well as a dot.
func parseWeightText(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return clampWeight(v), true
}

// parseBulk turns pasted text into entries, one per line. A trailing "| number"
// sets the weight; anything else is taken as part of the label.
//
//	Alice
//	Bob | 3
//	Long name with | a pipe in it
func parseBulk(text string) []store.Entry {
	lines := strings.Split(text, "\n")
	out := make([]store.Entry, 0, len(lines))

	for _, line := range lines {
		label, weight := line, 1.0
		if i := strings.LastIndex(line, "|"); i > 0 {
			if v, ok := parseWeightText(line[i+1:]); ok {
				label = line[:i]
				weight = v
			}
		}
		label = cleanLabel(label)
		if label == "" {
			continue
		}
		out = append(out, store.Entry{Label: label, Weight: weight})
	}
	return out
}
