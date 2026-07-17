package aesthetic

import "math"

// Score produces a 0..1 aesthetic quality estimate from visual metrics.
// It rewards sharpness and contrast, and a moderate (not extreme) brightness.
func Score(sharpness, contrast, brightness float64) float64 {
	sharp := clamp01(math.Sqrt(math.Max(sharpness, 0)) / 0.6)
	contr := clamp01(contrast * 3)
	// Prefer balanced brightness (penalize pure black/white extremes a little).
	brightBalance := 1 - math.Abs(brightness-0.5)*0.6
	brightBalance = clamp01(brightBalance)

	score := 0.45*sharp + 0.35*contr + 0.20*brightBalance
	return clamp01(score)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
