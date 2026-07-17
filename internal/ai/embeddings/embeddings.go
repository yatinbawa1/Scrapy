package embeddings

import (
	"image"
	"math"
	"strings"

	"wallpaper-chooser/internal/image/colors"
)

// AnalysisInput carries everything needed to build an image embedding. A real
// CLIP embedder would use Path/Image; the heuristic embedder uses the visual +
// text features so it works without a model download.
type AnalysisInput struct {
	Path           string
	Image          image.Image
	DominantColors []string
	Brightness     float64
	Contrast       float64
	Sharpness      float64
	Category       string
	Tags           []string
	Title          string
	SearchTerm     string
	// AutoLabels are image-derived concepts (logo, minimal, ...) computed during
	// analysis; CustomLabels are user-assigned tags (e.g. "Arch", "Nike").
	AutoLabels  []string
	CustomLabels []string
}

// Embedder turns images and text into vectors in a shared space so that cosine
// similarity is meaningful. The implementation is swappable (heuristic vs CLIP).
type Embedder interface {
	Dim() int
	EmbedText(text string) ([]float32, error)
	EmbedAnalysis(a AnalysisInput) ([]float32, error)
}

const (
	hueBins   = 16
	conceptN  = 45
	idxBright = 0
	idxContr  = 1
	idxSharp  = 2
	idxHue    = 3
	idxConc   = idxHue + hueBins
	// Dim = 3 + 16 + 45
	DimSize = idxConc + conceptN
)

// conceptList is the canonical concept vocabulary. The index aligns with the
// concept bins in the embedding, so adding brand/user tags that map to these
// words steers semantic search.
var conceptList = []string{
	"mountain", "space", "car", "cyberpunk", "anime", "minimal", "abstract",
	"architecture", "rain", "winter", "nature", "city", "beach", "people",
	"sunset", "forest", "animal", "ocean", "flower", "logo", "brand", "text",
	"typography", "geometric", "pattern", "gradient", "landscape", "sky",
	"cloud", "galaxy", "star", "urban", "building", "neon", "cartoon", "comic",
	"cat", "dog", "bird", "fish", "plant", "tree", "vehicle", "motorcycle",
	"portrait",
}

// synonyms -> concept index
var conceptSynonyms = map[string]int{}

func init() {
	for i, c := range conceptList {
		conceptSynonyms[c] = i
	}
	add := func(syn string, idx int) { conceptSynonyms[syn] = idx }
	add("mountains", 0)
	add("sky", 0)
	add("galaxy", 1)
	add("stars", 1)
	add("star", 30)
	add("vehicles", 2)
	add("vehicle", 42)
	add("cars", 2)
	add("snow", 9)
	add("landscape", 26)
	add("scenery", 26)
	add("sea", 17)
	add("water", 17)
	add("lake", 17)
	add("river", 17)
	add("cat", 36)
	add("dog", 37)
	add("wildlife", 16)
	add("plant", 40)
	add("buildings", 32)
	add("neon", 33)
	add("comics", 35)
	add("manga", 4)
	add("person", 13)
	add("face", 44)
	add("portrait", 44)
	add("logo", 19)
	add("logos", 19)
	add("brand", 20)
	add("brands", 20)
	add("wordmark", 19)
	add("letter", 21)
	add("letters", 21)
	add("font", 22)
	add("geometry", 23)
	add("shapes", 23)
	add("abstract", 6)
	add("art", 6)
	add("minimalist", 5)
	add("flat", 5)
}

// colorName -> hue degrees
var colorHue = map[string]float64{
	"red": 0, "orange": 30, "yellow": 50, "green": 130,
	"cyan": 180, "blue": 220, "purple": 280, "pink": 330, "magenta": 300,
}

// HeuristicEmbedder is the default, dependency-free embedder.
type HeuristicEmbedder struct{}

func (HeuristicEmbedder) Dim() int { return DimSize }

func (HeuristicEmbedder) EmbedText(text string) ([]float32, error) {
	v := newVector()
	lower := strings.ToLower(text)
	tokens := tokenizeText(lower)

	bright, contrast, sharp := 0.5, 0.3, 0.5
	hasBright := false

	for _, t := range tokens {
		if h, ok := colorHue[t]; ok {
			setHue(v, h, 1.0)
		}
		switch t {
		case "black", "dark", "amoled", "minimal", "minimalist", "flat":
			bright = 0.08
			hasBright = true
		case "white", "light":
			bright = 0.95
			hasBright = true
		}
		if idx, ok := conceptSynonyms[t]; ok {
			v[idxConc+idx] = 1.0
		}
	}
	if !hasBright {
		bright = 0.5
	}
	v[idxBright] = float32(bright)
	v[idxContr] = float32(contrast)
	v[idxSharp] = float32(sharp)
	normalize(v)
	return v, nil
}

func (h HeuristicEmbedder) EmbedAnalysis(a AnalysisInput) ([]float32, error) {
	v := newVector()

	bright := clamp01(a.Brightness)
	contrast := clamp01(a.Contrast * 3)
	sharp := clamp01(math.Sqrt(maxf(a.Sharpness, 0)) / 0.6)

	v[idxBright] = float32(bright)
	v[idxContr] = float32(contrast)
	v[idxSharp] = float32(sharp)

	// hue histogram from dominant colors
	for _, hex := range a.DominantColors {
		hdeg := hexHue(hex)
		if hdeg < 0 {
			continue
		}
		setHue(v, hdeg, 1.0)
	}

	// text-derived concepts (title, search term, tags, custom labels)
	corpus := strings.ToLower(strings.Join([]string{
		a.Category, strings.Join(a.Tags, " "), a.Title, a.SearchTerm,
		strings.Join(a.AutoLabels, " "), strings.Join(a.CustomLabels, " "),
	}, " "))
	for _, t := range tokenizeText(corpus) {
		if idx, ok := conceptSynonyms[t]; ok {
			v[idxConc+idx] = 1.0
		}
	}

	// image-derived concepts (logo / text / minimal / abstract / nature / city ...)
	for _, c := range ImageConcepts(a.Image, bright, sharp) {
		if idx, ok := conceptSynonyms[c]; ok {
			v[idxConc+idx] = 1.0
		}
	}

	normalize(v)
	return v, nil
}

// ImageConcepts inspects pixels and returns heuristic labels. This is how an
// untitled "Arch logo" wallpaper gets a "logo"/"text"/"brand" vector component
// even when nothing in its title mentions it.
func ImageConcepts(img image.Image, brightness, sharp float64) []string {
	if img == nil {
		return nil
	}
	edge, distinct := imageFeatures(img)
	var out []string
	switch {
	case distinct <= 3 && edge < 0.05:
		out = append(out, "gradient")
	case distinct <= 4 && edge >= 0.07:
		// few colors + crisp edges => text / logo / wordmark
		out = append(out, "logo", "text", "typography", "brand")
	case distinct <= 3:
		out = append(out, "minimal")
	case edge >= 0.09 && distinct <= 9:
		out = append(out, "geometric", "abstract", "pattern")
	case distinct >= 12 && edge < 0.09:
		out = append(out, "nature", "landscape")
	case edge >= 0.1:
		out = append(out, "city", "urban", "architecture", "building")
	}
	return out
}

// imageFeatures returns an edge-density estimate (0..1) and the number of
// distinct quantized colors in a downscaled view of the image.
func imageFeatures(img image.Image) (edgeDensity float64, distinct int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	sx, sy := w, h
	if sx > 160 {
		sx = 160
	}
	if sy > 160 {
		sy = 160
	}
	gray := make([][]float64, sy)
	buckets := make(map[uint32]struct{}, 64)
	for y := 0; y < sy; y++ {
		gray[y] = make([]float64, sx)
		for x := 0; x < sx; x++ {
			px := img.At(b.Min.X+x*w/sx, b.Min.Y+y*h/sy)
			r, g, bl, _ := px.RGBA()
			rr, gg, bb := uint8(r>>8), uint8(g>>8), uint8(bl>>8)
			gray[y][x] = 0.299*float64(rr) + 0.587*float64(gg) + 0.114*float64(bb)
			buckets[(uint32(rr>>4)<<8)|(uint32(gg>>4)<<4)|uint32(bb>>4)] = struct{}{}
		}
	}
	var sum float64
	var n int
	for y := 0; y < sy; y++ {
		for x := 0; x < sx; x++ {
			var d float64
			if x+1 < sx {
				d += math.Abs(gray[y][x] - gray[y][x+1])
			}
			if y+1 < sy {
				d += math.Abs(gray[y][x] - gray[y+1][x])
			}
			sum += d
			n += 2
		}
	}
	if n > 0 {
		edgeDensity = clamp01((sum / float64(n)) / 60.0)
	}
	distinct = len(buckets)
	return edgeDensity, distinct
}

// ---- helpers ----

func newVector() []float32 { return make([]float32, DimSize) }

func tokenizeText(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '-' || r == '.' || r == '/' || r == ':' || r == '(' || r == ')'
	})
}

func setHue(v []float32, hdeg float64, weight float64) {
	bin := int((hdeg / 360.0) * hueBins)
	if bin >= hueBins {
		bin = hueBins - 1
	}
	if bin < 0 {
		bin = 0
	}
	v[idxHue+bin] += float32(weight)
}

func normalize(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum <= 0 {
		return
	}
	norm := math.Sqrt(sum)
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func hexHue(hex string) float64 {
	r, g, b := colors.HexToRGBSafe(hex)
	if r < 0 {
		return -1
	}
	return rgbToHue(r, g, b)
}

func rgbToHue(r, g, b int) float64 {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	maxc, minc := rf, rf
	if gf > maxc {
		maxc = gf
	}
	if bf > maxc {
		maxc = bf
	}
	if gf < minc {
		minc = gf
	}
	if bf < minc {
		minc = bf
	}
	delta := maxc - minc
	if delta == 0 {
		return 0
	}
	var h float64
	switch maxc {
	case rf:
		h = 60 * math.Mod((gf-bf)/delta, 6)
	case gf:
		h = 60 * ((bf-rf)/delta + 2)
	default:
		h = 60 * ((rf-gf)/delta + 4)
	}
	if h < 0 {
		h += 360
	}
	return h
}

// Cosine returns the cosine similarity between two vectors. It tolerates
// different lengths (e.g. embeddings produced before the dimension changed) by
// comparing only the overlapping prefix.
func Cosine(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot, na, nb float64
	for i := 0; i < n; i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// ConceptOf returns the first canonical concept matched in text, or "".
func ConceptOf(text string) string {
	for _, t := range tokenizeText(strings.ToLower(text)) {
		if idx, ok := conceptSynonyms[t]; ok {
			return conceptList[idx]
		}
	}
	return ""
}
