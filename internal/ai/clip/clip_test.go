//go:build clip

package clip

import (
	"image"
	"image/color"
	"math"
	"os"
	"testing"

	"wallpaper-chooser/internal/ai/embeddings"
)

func cosine(a, b []float32) float32 {
	var dot, na, nb float32
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (float32(math.Sqrt(float64(na))) * float32(math.Sqrt(float64(nb))))
}

func TestClipSimilarity(t *testing.T) {
	dir := os.Getenv("CLIP_TEST_MODEL_DIR")
	if dir == "" {
		t.Skip("set CLIP_TEST_MODEL_DIR to the CLIP model directory")
	}
	e, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() {
		if c, ok := e.(interface{ Close() }); ok {
			c.Close()
		}
	}()

	mkImage := func(c color.RGBA) *image.RGBA {
		im := image.NewRGBA(image.Rect(0, 0, 224, 224))
		for y := 0; y < 224; y++ {
			for x := 0; x < 224; x++ {
				im.SetRGBA(x, y, c)
			}
		}
		return im
	}

	redVec, err := e.EmbedText("a red image")
	if err != nil {
		t.Fatalf("EmbedText red: %v", err)
	}
	blueVec, err := e.EmbedText("a blue image")
	if err != nil {
		t.Fatalf("EmbedText blue: %v", err)
	}

	for _, tc := range []struct {
		name   string
		img    color.RGBA
		match  []float32
		noMatch []float32
	}{
		{"red", color.RGBA{220, 30, 30, 255}, redVec, blueVec},
		{"blue", color.RGBA{30, 60, 220, 255}, blueVec, redVec},
	} {
		iv, err := e.EmbedAnalysis(embeddings.AnalysisInput{Image: mkImage(tc.img)})
		if err != nil {
			t.Fatalf("EmbedAnalysis %s: %v", tc.name, err)
		}
		if len(iv) != clipDim {
			t.Fatalf("expected dim %d, got %d", clipDim, len(iv))
		}
		simMatch := cosine(iv, tc.match)
		simNo := cosine(iv, tc.noMatch)
		t.Logf("%s image: match=%.3f nomatch=%.3f", tc.name, simMatch, simNo)
		if simMatch <= simNo {
			t.Errorf("%s image closer to wrong color (match=%.3f nomatch=%.3f)", tc.name, simMatch, simNo)
		}
	}
}
