package metadata

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	_ "golang.org/x/image/webp"
)

// Info describes basic file/format metadata for an image.
type Info struct {
	Width  int
	Height int
	Format string
	Size   int64
}

// Extract reads an image file and returns its dimensions, detected format and
// byte size. The format is detected from magic bytes (jpeg/png/gif/webp).
func Extract(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return Info{}, err
	}

	var buf [12]byte
	n, _ := f.Read(buf[:])
	if n == 0 {
		return Info{}, fmt.Errorf("empty file: %s", path)
	}
	format := detectFormat(buf[:n])

	if _, err := f.Seek(0, 0); err != nil {
		return Info{}, err
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return Info{}, fmt.Errorf("decode config %s: %w", path, err)
	}

	return Info{
		Width:  cfg.Width,
		Height: cfg.Height,
		Format: format,
		Size:   info.Size(),
	}, nil
}

func detectFormat(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "jpeg"
	case len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47:
		return "png"
	case len(b) >= 6 && string(b[:6]) == "GIF89a", len(b) >= 6 && string(b[:6]) == "GIF87a":
		return "gif"
	case len(b) >= 4 && b[0] == 'R' && b[1] == 'I' && b[2] == 'F' && b[3] == 'F':
		return "webp"
	}
	return "unknown"
}

// AspectRatio returns width/height (landscape > 1, portrait < 1).
func AspectRatio(w, h int) float64 {
	if h == 0 {
		return 0
	}
	return float64(w) / float64(h)
}

// Decode loads a full image for analysis (colors/quality). The caller is
// responsible for closing the returned image (it is safe to ignore for our use
// since Go's GC reclaims it, but Decode returns the *image.Image for callers
// that want to inspect pixels).
func Decode(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", path, err)
	}
	return img, strings.ToLower(format), nil
}
