//go:build clip

// Package clip provides a real CLIP-backed embeddings.Embedder using ONNX
// Runtime. When built without the "clip" build tag this file is excluded and
// the application falls back to the dependency-free heuristic embedder.
package clip

import (
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"

	ort "github.com/yalue/onnxruntime_go"

	"wallpaper-chooser/internal/ai/embeddings"
)

const clipDim = 512

// ErrNotEnabled is returned when the CLIP model files are not present.
var ErrNotEnabled = errors.New("clip: model not configured (using heuristic embedder)")

// ClipEmbedder satisfies embeddings.Embedder using CLIP vision/text encoders.
type ClipEmbedder struct {
	visionSession *ort.DynamicAdvancedSession
	textSession   *ort.DynamicAdvancedSession
	tokenizer     *Tokenizer
}

// New loads the CLIP ONNX models from modelDir (vision_model.onnx,
// text_model.onnx, vocab.json, merges.txt) and the ONNX runtime shared
// library. It returns ErrNotEnabled if the model files are missing.
func New(modelDir string) (embeddings.Embedder, error) {
	if modelDir == "" {
		return nil, ErrNotEnabled
	}
	vp := filepath.Join(modelDir, "vision_model.onnx")
	tp := filepath.Join(modelDir, "text_model.onnx")
	vocab := filepath.Join(modelDir, "vocab.json")
	merges := filepath.Join(modelDir, "merges.txt")
	if !exists(vp) || !exists(tp) || !exists(vocab) || !exists(merges) {
		return nil, ErrNotEnabled
	}
	if lib := findLib(modelDir); lib != "" {
		ort.SetSharedLibraryPath(lib)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, err
	}

	tok, err := NewTokenizer(vocab, merges)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, err
	}
	vs, err := ort.NewDynamicAdvancedSession(vp, []string{"pixel_values"}, []string{"image_embeds"}, nil)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, err
	}
	ts, err := ort.NewDynamicAdvancedSession(tp, []string{"input_ids", "attention_mask"}, []string{"text_embeds"}, nil)
	if err != nil {
		vs.Destroy()
		ort.DestroyEnvironment()
		return nil, err
	}
	return &ClipEmbedder{visionSession: vs, textSession: ts, tokenizer: tok}, nil
}

// Dim returns the CLIP embedding dimension.
func (c *ClipEmbedder) Dim() int { return clipDim }

// EmbedText encodes a natural-language query into a CLIP text embedding.
func (c *ClipEmbedder) EmbedText(text string) ([]float32, error) {
	ids := c.tokenizer.Encode(text)
	inputIDs := make([]int64, 77)
	attn := make([]int64, 77)
	for i, v := range ids {
		inputIDs[i] = int64(v)
		attn[i] = 1
	}
	idT, err := ort.NewTensor(ort.NewShape(1, 77), inputIDs)
	if err != nil {
		return nil, err
	}
	defer idT.Destroy()
	attT, err := ort.NewTensor(ort.NewShape(1, 77), attn)
	if err != nil {
		return nil, err
	}
	defer attT.Destroy()

	out := make([]float32, clipDim)
	outT, err := ort.NewTensor(ort.NewShape(1, clipDim), out)
	if err != nil {
		return nil, err
	}
	defer outT.Destroy()

	if err := c.textSession.Run([]ort.Value{idT, attT}, []ort.Value{outT}); err != nil {
		return nil, err
	}
	vec := outT.GetData()
	normalize(vec)
	return vec, nil
}

// EmbedAnalysis encodes an image into a CLIP vision embedding. When no image
// is available (e.g. re-embedding from labels), it falls back to encoding the
// wallpaper's text metadata so the vector stays in the shared space.
func (c *ClipEmbedder) EmbedAnalysis(a embeddings.AnalysisInput) ([]float32, error) {
	if a.Image != nil {
		data := preprocess(a.Image)
		t, err := ort.NewTensor(ort.NewShape(1, 3, 224, 224), data)
		if err != nil {
			return nil, err
		}
		defer t.Destroy()
		out := make([]float32, clipDim)
		outT, err := ort.NewTensor(ort.NewShape(1, clipDim), out)
		if err != nil {
			return nil, err
		}
		defer outT.Destroy()
		if err := c.visionSession.Run([]ort.Value{t}, []ort.Value{outT}); err != nil {
			return nil, err
		}
		vec := outT.GetData()
		normalize(vec)
		return vec, nil
	}

	text := strings.Join(a.Tags, " ")
	if a.Title != "" {
		text = a.Title + " " + text
	}
	if a.SearchTerm != "" {
		text = a.SearchTerm + " " + text
	}
	text = text + " " + strings.Join(a.AutoLabels, " ") + " " + strings.Join(a.CustomLabels, " ")
	return c.EmbedText(strings.TrimSpace(text))
}

// Close releases the ONNX sessions and the runtime environment.
func (c *ClipEmbedder) Close() {
	if c.visionSession != nil {
		c.visionSession.Destroy()
	}
	if c.textSession != nil {
		c.textSession.Destroy()
	}
	ort.DestroyEnvironment()
}

// ---- image preprocessing (CLIP normalization) ----

func preprocess(img image.Image) []float32 {
	const size = 224
	resized := resizeImage(img, size, size)
	mean := [3]float32{0.48145466, 0.4578275, 0.40821073}
	std := [3]float32{0.26862954, 0.26130258, 0.27577711}
	data := make([]float32, 3*size*size)
	idx := 0
	for c := 0; c < 3; c++ {
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				r, g, b := sampleRGB(resized, x, y)
				var v float32
				switch c {
				case 0:
					v = (r - mean[0]) / std[0]
				case 1:
					v = (g - mean[1]) / std[1]
				case 2:
					v = (b - mean[2]) / std[2]
				}
				data[idx] = v
				idx++
			}
		}
	}
	return data
}

func sampleRGB(img *image.RGBA, x, y int) (float32, float32, float32) {
	px := img.RGBAAt(x, y)
	return float32(px.R) / 255.0, float32(px.G) / 255.0, float32(px.B) / 255.0
}

func resizeImage(img image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw == 0 || ih == 0 {
		return dst
	}
	for y := 0; y < h; y++ {
		fy := (float64(y) + 0.5) * float64(ih) / float64(h)
		y0 := int(fy)
		if y0 >= ih {
			y0 = ih - 1
		}
		y1 := y0 + 1
		if y1 >= ih {
			y1 = ih - 1
		}
		for x := 0; x < w; x++ {
			fx := (float64(x) + 0.5) * float64(iw) / float64(w)
			x0 := int(fx)
			if x0 >= iw {
				x0 = iw - 1
			}
			x1 := x0 + 1
			if x1 >= iw {
				x1 = iw - 1
			}
			c00 := rgbaAt(img, b.Min.X+x0, b.Min.Y+y0)
			c10 := rgbaAt(img, b.Min.X+x1, b.Min.Y+y0)
			c01 := rgbaAt(img, b.Min.X+x0, b.Min.Y+y1)
			c11 := rgbaAt(img, b.Min.X+x1, b.Min.Y+y1)
			tx := fx - float64(x0)
			ty := fy - float64(y0)
			r := lerp(lerp(uint32(c00.R), uint32(c10.R), tx), lerp(uint32(c01.R), uint32(c11.R), tx), ty)
			g := lerp(lerp(uint32(c00.G), uint32(c10.G), tx), lerp(uint32(c01.G), uint32(c11.G), tx), ty)
			bl := lerp(lerp(uint32(c00.B), uint32(c10.B), tx), lerp(uint32(c01.B), uint32(c11.B), tx), ty)
			dst.SetRGBA(x, y, color.RGBA{uint8(clamp8(r)), uint8(clamp8(g)), uint8(clamp8(bl)), 255})
		}
	}
	return dst
}

func rgbaAt(img image.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}

func lerp(a, b uint32, t float64) uint32 {
	return uint32(float64(a) + t*float64(int32(b)-int32(a)))
}

func clamp8(v uint32) uint32 {
	if v > 255 {
		return 255
	}
	return v
}

func normalize(v []float32) {
	var n float32
	for _, x := range v {
		n += x * x
	}
	n = float32(math.Sqrt(float64(n)))
	if n > 0 {
		for i := range v {
			v[i] /= n
		}
	}
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func findLib(dir string) string {
	cands := []string{
		os.Getenv("ORT_LIB_PATH"),
		filepath.Join(dir, "libonnxruntime.dylib"),
		filepath.Join(dir, "libonnxruntime.so"),
		filepath.Join(dir, "onnxruntime.dll"),
	}
	for _, c := range cands {
		if c != "" && exists(c) {
			return c
		}
	}
	return ""
}
