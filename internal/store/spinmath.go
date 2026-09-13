package store

import (
	"math"

	"github.com/Alexander-D-Karpov/wheel/internal/idgen"
)

const (
	tau = 2 * math.Pi

	// pointerAngle is where the fixed marker sits in canvas coordinates:
	// straight up, since canvas angle 0 points right and grows clockwise.
	pointerAngle = -math.Pi / 2

	// Turn count scales with the spin duration so a long spin does not crawl,
	// plus a random extra turn or two: the distance travelled must never hint
	// at where the wheel is going to stop.
	turnsPerSecond = 0.9
	minTurns       = 5
	maxTurns       = 40
	extraTurns     = 4

	// edgeMargin keeps the pointer away from the seam between two slices, where
	// rounding in the browser could make it look like it stopped on the wrong one.
	edgeMargin = 0.12
)

// sliceWeight mirrors the browser's guard so both sides divide the circle the
// same way even if a weight ever slips through as zero.
func sliceWeight(e PlayEntry) float64 {
	if e.Weight > 0 {
		return e.Weight
	}
	return 0.01
}

func totalWeight(entries []PlayEntry) float64 {
	total := 0.0
	for _, e := range entries {
		total += sliceWeight(e)
	}
	if total <= 0 {
		return float64(len(entries))
	}
	return total
}

// sliceBounds returns the start and end angle of entry i in wheel-local
// radians, before any rotation is applied.
func sliceBounds(entries []PlayEntry, i int) (float64, float64) {
	total := totalWeight(entries)
	start := 0.0
	for k := 0; k < i; k++ {
		start += sliceWeight(entries[k])
	}
	return start / total * tau, (start + sliceWeight(entries[i])) / total * tau
}

// pickWinner chooses an index with probability proportional to weight.
func pickWinner(entries []PlayEntry) int {
	if len(entries) == 0 {
		return -1
	}
	total := totalWeight(entries)
	target := idgen.Float() * total
	acc := 0.0
	for i, e := range entries {
		acc += sliceWeight(e)
		if target < acc {
			return i
		}
	}
	return len(entries) - 1
}

// turnsFor picks how many whole turns a spin of the given length should make.
func turnsFor(seconds int) float64 {
	turns := int(math.Round(float64(seconds) * turnsPerSecond))
	if turns < minTurns {
		turns = minTurns
	}
	if turns > maxTurns {
		turns = maxTurns
	}
	return float64(turns + idgen.Int(extraTurns))
}

// spinDelta returns how far the wheel must travel, starting from rotation
// `from`, to leave the winner's slice under the pointer. The result always
// includes several whole turns so the motion looks like a spin rather than a
// nudge, and the landing point inside the slice is randomised.
func spinDelta(entries []PlayEntry, winner int, from float64, seconds int) float64 {
	lo, hi := sliceBounds(entries, winner)
	span := hi - lo
	margin := span * edgeMargin
	target := lo + margin + idgen.Float()*(span-2*margin)

	// Final rotation R satisfies R + target == pointerAngle (mod tau).
	base := math.Mod(pointerAngle-target-from, tau)
	if base < 0 {
		base += tau
	}
	return turnsFor(seconds)*tau + base
}

// normalizeRotation folds an angle into [0, tau) so stored rotations stay small
// across long sessions.
func normalizeRotation(r float64) float64 {
	r = math.Mod(r, tau)
	if r < 0 {
		r += tau
	}
	return r
}

// entryUnderPointer reports which slice the pointer indicates at rotation r.
// It exists so the spin maths can be asserted in tests.
func entryUnderPointer(entries []PlayEntry, r float64) int {
	local := normalizeRotation(pointerAngle - r)
	for i := range entries {
		lo, hi := sliceBounds(entries, i)
		if local >= lo && local < hi {
			return i
		}
	}
	return len(entries) - 1
}
