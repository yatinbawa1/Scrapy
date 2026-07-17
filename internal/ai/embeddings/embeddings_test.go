package embeddings

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// makeLogoTestImage returns a small image with a bright rectangle on a dark
// background: few colors + crisp edges, which should read as logo/text/minimal.
func makeLogoTestImage(size int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	// dark background
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, color.RGBA{20, 20, 20, 255})
		}
	}
	// white center square -> sharp edges, 2 colors
	s := size / 2
	for y := s / 2; y < s+s/2; y++ {
		for x := s / 2; x < s+s/2; x++ {
			img.Set(x, y, color.RGBA{240, 240, 240, 255})
		}
	}
	return img
}

func TestCustomLabelSearch(t *testing.T) {
	var e HeuristicEmbedder
	q, err := e.EmbedText("logo brand")
	if err != nil {
		t.Fatalf("EmbedText: %v", err)
	}

	// A wallpaper with NO text metadata, only a user tag "logo" (a known concept).
	tagged, err := e.EmbedAnalysis(AnalysisInput{CustomLabels: []string{"logo"}})
	if err != nil {
		t.Fatalf("EmbedAnalysis: %v", err)
	}

	// A control wallpaper with unrelated tags.
	other, err := e.EmbedAnalysis(AnalysisInput{CustomLabels: []string{"forest", "mountain"}})
	if err != nil {
		t.Fatalf("EmbedAnalysis: %v", err)
	}

	simTagged := Cosine(q, tagged)
	simOther := Cosine(q, other)

	t.Logf("query 'logo brand' ~ tagged[logo]=%.3f  ~ other[forest]=%.3f", simTagged, simOther)
	if simTagged < 0.5 {
		t.Errorf("expected strong match for custom label 'logo', got %.3f", simTagged)
	}
	if simOther >= simTagged {
		t.Errorf("unrelated wallpaper should be less similar (tagged=%.3f other=%.3f)", simTagged, simOther)
	}
	if math.IsNaN(simTagged) || math.IsNaN(simOther) {
		t.Fatal("NaN similarity")
	}
}

func TestImageConceptsLogo(t *testing.T) {
	// Build a tiny synthetic "logo-like" image: 2 colors, crisp edges.
	img := makeLogoTestImage(32)
	concepts := ImageConcepts(img, 0.15, 0.9)
	t.Logf("concepts: %v", concepts)
	if !contains(concepts, "logo") && !contains(concepts, "text") && !contains(concepts, "minimal") {
		t.Errorf("expected logo/text/minimal concept for flat low-color image, got %v", concepts)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
