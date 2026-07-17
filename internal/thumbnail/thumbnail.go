package thumbnail

import (
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"

	_ "golang.org/x/image/webp"

	"github.com/nfnt/resize"
)

const (
	ThumbWidth  = 512
	ThumbHeight = 0
	// maxDim guards against decoding pathologically large images into memory,
	// which can exhaust RAM and crash the app.
	maxDim = 8000
)

type Generator struct {
	thumbDir string
}

func New(thumbDir string) *Generator {
	return &Generator{thumbDir: thumbDir}
}

func (g *Generator) GenerateIfMissing(localPath, thumbPath string) error {
	if info, err := os.Stat(thumbPath); err == nil && info.Size() > 1024 {
		return nil
	}
	os.Remove(thumbPath)
	return g.Generate(localPath, thumbPath)
}

func (g *Generator) GenerateWithBrightness(src, dst string) (float64, error) {
	f, err := os.Open(src)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", src, err)
	}
	defer f.Close()

	cfg, _, err := image.DecodeConfig(f)
	if err == nil && (cfg.Width > maxDim || cfg.Height > maxDim) {
		return 0, fmt.Errorf("image too large to thumbnail: %dx%d", cfg.Width, cfg.Height)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}

	img, _, err := image.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("decode %s: %w", src, err)
	}

	brightness := computeBrightness(img)

	thumb := resize.Thumbnail(ThumbWidth, ThumbHeight, img, resize.Lanczos3)

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return 0, err
	}

	out, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer out.Close()

	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: 85}); err != nil {
		return 0, err
	}

	return brightness, nil
}

func (g *Generator) Generate(src, dst string) error {
	_, err := g.GenerateWithBrightness(src, dst)
	return err
}

func computeBrightness(img image.Image) float64 {
	bounds := img.Bounds()
	w, h := bounds.Max.X-bounds.Min.X, bounds.Max.Y-bounds.Min.Y
	if w == 0 || h == 0 {
		return 0.5
	}

	step := 1
	if w > 100 || h > 100 {
		step = 4
	}

	var totalLuma float64
	var count float64

	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, _ := img.At(x, y).RGBA()
			luma := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
			totalLuma += luma
			count++
		}
	}

	if count == 0 {
		return 0.5
	}
	return totalLuma / count
}

func ComputeBrightness(img image.Image) float64 {
	return computeBrightness(img)
}

func (g *Generator) ComputeBrightnessFromFile(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return 0, err
	}

	return computeBrightness(img), nil
}
