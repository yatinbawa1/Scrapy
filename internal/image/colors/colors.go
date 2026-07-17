package colors

import (
	"image"
	"sort"
	"strings"

	"github.com/nfnt/resize"
)

type box struct {
	pixels []pixel
}

type pixel struct {
	r, g, b uint8
}

// DominantColors computes up to `count` dominant colors of an image using the
// median-cut algorithm. The image is downscaled first for performance.
func DominantColors(img image.Image, count int) ([]string, error) {
	if count <= 0 {
		count = 5
	}
	small := resize.Thumbnail(64, 64, img, resize.NearestNeighbor)

	b := box{}
	bounds := small.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, bl, _ := small.At(x, y).RGBA()
			b.pixels = append(b.pixels, pixel{
				r: uint8(r >> 8),
				g: uint8(g >> 8),
				b: uint8(bl >> 8),
			})
		}
	}
	if len(b.pixels) == 0 {
		return nil, nil
	}

	boxes := medianCut(b, count)
	type weighted struct {
		hex string
		pop int
	}
	var ws []weighted
	for _, bx := range boxes {
		if len(bx.pixels) == 0 {
			continue
		}
		var r, g, bl uint32
		for _, p := range bx.pixels {
			r += uint32(p.r)
			g += uint32(p.g)
			bl += uint32(p.b)
		}
		n := uint32(len(bx.pixels))
		hex := rgbToHex(uint8(r/n), uint8(g/n), uint8(bl/n))
		ws = append(ws, weighted{hex: hex, pop: len(bx.pixels)})
	}
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].pop > ws[j].pop })

	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.hex)
	}
	return out, nil
}

func medianCut(b box, count int) []box {
	boxes := []box{b}
	for len(boxes) < count {
		// Find the box with the largest color range to split.
		idx := -1
		var maxRange int
		for i, bx := range boxes {
			if r := boxRange(bx); r > maxRange {
				maxRange = r
				idx = i
			}
		}
		if idx == -1 || len(boxes[idx].pixels) < 2 {
			break
		}
		splitBox := boxes[idx]
		ch := longestChannel(splitBox)
		sort.Slice(splitBox.pixels, func(i, j int) bool {
			switch ch {
			case 0:
				return splitBox.pixels[i].r < splitBox.pixels[j].r
			case 1:
				return splitBox.pixels[i].g < splitBox.pixels[j].g
			default:
				return splitBox.pixels[i].b < splitBox.pixels[j].b
			}
		})
		mid := len(splitBox.pixels) / 2
		left := box{pixels: splitBox.pixels[:mid]}
		right := box{pixels: splitBox.pixels[mid:]}
		boxes = append(boxes[:idx], append([]box{left, right}, boxes[idx+1:]...)...)
	}
	return boxes
}

func boxRange(b box) int {
	if len(b.pixels) == 0 {
		return 0
	}
	var rmin, rmax, gmin, gmax, bmin, bmax uint8
	rmin, gmin, bmin = 255, 255, 255
	for _, p := range b.pixels {
		if p.r < rmin {
			rmin = p.r
		}
		if p.r > rmax {
			rmax = p.r
		}
		if p.g < gmin {
			gmin = p.g
		}
		if p.g > gmax {
			gmax = p.g
		}
		if p.b < bmin {
			bmin = p.b
		}
		if p.b > bmax {
			bmax = p.b
		}
	}
	r := int(rmax) - int(rmin)
	g := int(gmax) - int(gmin)
	bl := int(bmax) - int(bmin)
	if r > g && r > bl {
		return r
	}
	if g > bl {
		return g
	}
	return bl
}

func longestChannel(b box) int {
	var rmin, rmax, gmin, gmax, bmin, bmax uint8
	rmin, gmin, bmin = 255, 255, 255
	for _, p := range b.pixels {
		if p.r < rmin {
			rmin = p.r
		}
		if p.r > rmax {
			rmax = p.r
		}
		if p.g < gmin {
			gmin = p.g
		}
		if p.g > gmax {
			gmax = p.g
		}
		if p.b < bmin {
			bmin = p.b
		}
		if p.b > bmax {
			bmax = p.b
		}
	}
	r := int(rmax) - int(rmin)
	g := int(gmax) - int(gmin)
	bl := int(bmax) - int(bmin)
	if r >= g && r >= bl {
		return 0
	}
	if g >= bl {
		return 1
	}
	return 2
}

// AverageLuminance returns the mean luminance (0..1) of an image, used as a
// brightness estimate.
func AverageLuminance(img image.Image) float64 {
	bounds := img.Bounds()
	var total float64
	var n int
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 2 {
			r, g, b, _ := img.At(x, y).RGBA()
			total += (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
			n++
		}
	}
	if n == 0 {
		return 0.5
	}
	return total / float64(n)
}

func rgbToHex(r, g, b uint8) string {
	return "#" + hex2(r) + hex2(g) + hex2(b)
}

func hex2(v uint8) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>4], digits[v&0x0f]})
}

// NearestColorDistance returns the squared Euclidean distance between two hex
// colors in RGB space.
func NearestColorDistance(a, b string) float64 {
	ar, ag, ab := hexToRGB(a)
	br, bg, bb := hexToRGB(b)
	dr := float64(ar - br)
	dg := float64(ag - bg)
	db := float64(ab - bb)
	return dr*dr + dg*dg + db*db
}

func hexToRGB(h string) (int, int, int) {
	h = strings.TrimPrefix(h, "#")
	if len(h) < 6 {
		return 0, 0, 0
	}
	r := atoi16(h[0:2])
	g := atoi16(h[2:4])
	b := atoi16(h[4:6])
	return r, g, b
}

// ColorName returns a rough human-readable name for a hex color (blue, purple, ...).
func ColorName(h string) string {
	r, g, b := HexToRGBSafe(h)
	if r < 0 {
		return ""
	}
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
	lum := (maxc + minc) / 2
	if lum < 0.12 {
		return "black"
	}
	if lum > 0.9 {
		return "white"
	}
	if maxc-minc < 0.08 {
		if lum < 0.5 {
			return "gray"
		}
		return "gray"
	}
	hdeg := rgbToHue(r, g, b)
	switch {
	case hdeg < 15 || hdeg >= 345:
		return "red"
	case hdeg < 45:
		return "orange"
	case hdeg < 70:
		return "yellow"
	case hdeg < 165:
		return "green"
	case hdeg < 195:
		return "cyan"
	case hdeg < 255:
		return "blue"
	case hdeg < 290:
		return "purple"
	case hdeg < 345:
		return "pink"
	}
	return "gray"
}

// HexToRGBSafe parses a hex color (#rrggbb) and returns (-1,-1,-1) if invalid.
func HexToRGBSafe(h string) (int, int, int) {
	h = strings.TrimPrefix(h, "#")
	if len(h) < 6 {
		return -1, -1, -1
	}
	r := atoi16(h[0:2])
	g := atoi16(h[2:4])
	b := atoi16(h[4:6])
	return r, g, b
}

// rgbToHue returns the hue in degrees [0,360) for an RGB triple.
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
		h = ((gf - bf) / delta)
		if h < 0 {
			h += 6
		}
		h *= 60
	case gf:
		h = ((bf-rf)/delta + 2) * 60
	default:
		h = ((rf-gf)/delta + 4) * 60
	}
	if h >= 360 {
		h -= 360
	}
	return h
}

func atoi16(s string) int {
	var v int
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'a' && c <= 'f':
			v |= int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= int(c-'A') + 10
		}
	}
	return v
}
