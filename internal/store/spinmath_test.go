package store

import (
	"math"
	"testing"
)

func entries(weights ...float64) []PlayEntry {
	out := make([]PlayEntry, len(weights))
	for i, w := range weights {
		out[i] = PlayEntry{ID: string(rune('a' + i)), Weight: w, Position: i, Active: true}
	}
	return out
}

func TestSpinDeltaLandsOnWinner(t *testing.T) {
	cases := [][]float64{
		{1, 1},
		{1, 1, 1, 1, 1},
		{5, 1, 0.5, 12, 3, 1},
		{1, 100},
	}

	for _, weights := range cases {
		list := entries(weights...)
		for winner := range list {
			for attempt := 0; attempt < 200; attempt++ {
				from := idgenFloatSeeded(attempt) * tau
				seconds := 3 + attempt%60
				delta := spinDelta(list, winner, from, seconds)

				if delta < minTurns*tau {
					t.Fatalf("delta %.3f is under the %d turn minimum", delta, minTurns)
				}
				if got := entryUnderPointer(list, from+delta); got != winner {
					t.Fatalf("weights %v: wanted slice %d under the pointer, got %d", weights, winner, got)
				}
			}
		}
	}
}

func TestSliceBoundsCoverTheCircle(t *testing.T) {
	list := entries(3, 1, 1, 0.25)
	_, last := sliceBounds(list, len(list)-1)
	if math.Abs(last-tau) > 1e-9 {
		t.Fatalf("slices end at %.12f, want %.12f", last, tau)
	}
	for i := 1; i < len(list); i++ {
		_, prevHi := sliceBounds(list, i-1)
		lo, _ := sliceBounds(list, i)
		if math.Abs(prevHi-lo) > 1e-12 {
			t.Fatalf("gap between slice %d and %d: %.12f vs %.12f", i-1, i, prevHi, lo)
		}
	}
}

func TestPickWinnerRespectsWeights(t *testing.T) {
	// A weight of zero can never be selected, and a lone positive weight always is.
	list := []PlayEntry{{ID: "a", Weight: 0.01}, {ID: "b", Weight: 10_000}}
	hitsA := 0
	for i := 0; i < 5_000; i++ {
		if pickWinner(list) == 0 {
			hitsA++
		}
	}
	if hitsA > 50 {
		t.Fatalf("the 0.01 weight entry won %d of 5000 spins, expected roughly 5", hitsA)
	}
}

func TestNormalizeRotation(t *testing.T) {
	for _, in := range []float64{-7 * tau, -0.5, 0, 0.5, 9 * tau, 3.7} {
		got := normalizeRotation(in)
		if got < 0 || got >= tau {
			t.Fatalf("normalizeRotation(%.3f) = %.3f, outside [0, tau)", in, got)
		}
		if math.Abs(math.Mod(got-in, tau)) > 1e-9 && math.Abs(math.Abs(math.Mod(got-in, tau))-tau) > 1e-9 {
			t.Fatalf("normalizeRotation(%.3f) = %.3f changed the angle", in, got)
		}
	}
}

// idgenFloatSeeded gives the table above a spread of deterministic start angles.
func idgenFloatSeeded(i int) float64 {
	return math.Mod(float64(i)*0.6180339887498949, 1)
}
