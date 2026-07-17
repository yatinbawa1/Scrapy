package quality

import (
	"image"
	"math"
)

// Metrics holds the computed visual quality metrics for an image.
type Metrics struct {
	Brightness float64 // 0..1 mean luminance
	Contrast   float64 // RMS contrast (0..1)
	Sharpness  float64 // variance of Laplacian (higher = sharper)
}

// Analyze computes brightness, contrast and a sharpness/blur score for img.
func Analyze(img image.Image) Metrics {
	bounds := img.Bounds()
	w, h := bounds.Max.X-bounds.Min.X, bounds.Max.Y-bounds.Min.Y
	if w == 0 || h == 0 {
		return Metrics{}
	}

	gray := make([][]float64, h)
	for y := 0; y < h; y++ {
		gray[y] = make([]float64, w)
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			// Rec. 601 luma
			l := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
			gray[y][x] = l
		}
	}

	var sum float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			sum += gray[y][x]
		}
	}
	mean := sum / float64(w*h)

	var varSum float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := gray[y][x] - mean
			varSum += d * d
		}
	}
	contrast := math.Sqrt(varSum / float64(w*h))

	// Variance of Laplacian (sharpeness / blur estimation).
	var lapSum, lapSumSq float64
	var lapN int
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			lap := 4*gray[y][x] - gray[y-1][x] - gray[y+1][x] - gray[y][x-1] - gray[y][x+1]
			lapSum += lap
			lapSumSq += lap * lap
			lapN++
		}
	}
	var sharpness float64
	if lapN > 0 {
		m := lapSum / float64(lapN)
		sharpness = lapSumSq/float64(lapN) - m*m
		if sharpness < 0 {
			sharpness = 0
		}
	}

	return Metrics{
		Brightness: mean,
		Contrast:   contrast,
		Sharpness:  sharpness,
	}
}

// IsDark reports whether the image is predominantly dark (for AMOLED/Dark collections).
func IsDark(m Metrics) bool { return m.Brightness < 0.25 }

// IsLight reports whether the image is predominantly light.
func IsLight(m Metrics) bool { return m.Brightness > 0.75 }

// Normalize returns metrics scaled to 0..1 for embedding/storage.
func (m Metrics) Normalize() (brightness, contrast, sharpness float64) {
	brightness = clamp01(m.Brightness)
	contrast = clamp01(m.Contrast * 3) // RMS contrast is usually small
	sharpness = clamp01(math.Sqrt(m.Sharpness) / 0.6)
	return
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
