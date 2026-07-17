package duplicate

import (
	"image"
	"math"
	"sort"

	"github.com/nfnt/resize"
)

const dctSize = 32
const hashSize = 8

// Hash computes a 64-bit perceptual hash (as a 16-char hex string) of an image
// using a DCT (discrete cosine transform) of its luma, following the standard
// pHash approach.
func Hash(img image.Image) (string, error) {
	small := resize.Resize(dctSize, dctSize, img, resize.Bilinear)

	luma := make([][]float64, dctSize)
	for y := 0; y < dctSize; y++ {
		luma[y] = make([]float64, dctSize)
		for x := 0; x < dctSize; x++ {
			r, g, b, _ := small.At(x, y).RGBA()
			luma[y][x] = (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 65535.0
		}
	}

	// DCT of the top-left hashSize x hashSize low-frequency block.
	coeff := make([][]float64, hashSize)
	for i := 0; i < hashSize; i++ {
		coeff[i] = make([]float64, hashSize)
		for j := 0; j < hashSize; j++ {
			var sum float64
			for x := 0; x < dctSize; x++ {
				for y := 0; y < dctSize; y++ {
					sum += luma[y][x] *
						math.Cos(math.Pi*float64(2*x+1)*float64(i)/float64(2*dctSize)) *
						math.Cos(math.Pi*float64(2*y+1)*float64(j)/float64(2*dctSize))
				}
			}
			coeff[i][j] = sum
		}
	}

	// Median of the coefficients (excluding the DC term at 0,0).
	vals := make([]float64, 0, hashSize*hashSize-1)
	for i := 0; i < hashSize; i++ {
		for j := 0; j < hashSize; j++ {
			if i == 0 && j == 0 {
				continue
			}
			vals = append(vals, coeff[i][j])
		}
	}
	median := medianOf(vals)

	var bits [hashSize * hashSize]uint8
	idx := 0
	for i := 0; i < hashSize; i++ {
		for j := 0; j < hashSize; j++ {
			if i == 0 && j == 0 {
				bits[idx] = 0
			} else if coeff[i][j] >= median {
				bits[idx] = 1
			}
			idx++
		}
	}

	return bitsToHex(bits[:]), nil
}

// HammingDistance returns the number of differing bits between two pHash hex strings.
// Lower means more similar (0 = identical).
func HammingDistance(a, b string) int {
	if len(a) != len(b) {
		return 64
	}
	var dist int
	for i := 0; i < len(a); i++ {
		ca, cb := hexVal(a[i]), hexVal(b[i])
		x := ca ^ cb
		for x != 0 {
			dist += int(x & 1)
			x >>= 1
		}
	}
	return dist
}

func bitsToHex(bits []uint8) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(bits)/4)
	for i := 0; i < len(bits); i += 4 {
		var v uint8
		for j := 0; j < 4; j++ {
			v = (v << 1) | bits[i+j]
		}
		out = append(out, digits[v])
	}
	return string(out)
}

func medianOf(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sorted := append([]float64(nil), v...)
	sort.Float64s(sorted)
	return sorted[len(sorted)/2]
}

func hexVal(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
