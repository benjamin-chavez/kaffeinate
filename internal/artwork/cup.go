package artwork

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

func CupPNG(size int, active bool) []byte {
	iconImage := image.NewNRGBA(image.Rect(0, 0, size, size))
	segments := cupSegments(active)
	for y := range size {
		for x := range size {
			coverage := 0
			for sy := range 4 {
				for sx := range 4 {
					px := (float64(x) + (float64(sx)+0.5)/4) * 32 / float64(size)
					py := (float64(y) + (float64(sy)+0.5)/4) * 32 / float64(size)
					if cupContains(px, py, segments) {
						coverage++
					}
				}
			}
			iconImage.SetNRGBA(x, y, color.NRGBA{A: uint8(coverage * 255 / 16)})
		}
	}
	var imageBuffer bytes.Buffer
	_ = png.Encode(&imageBuffer, iconImage)
	return imageBuffer.Bytes()
}

func cupSegments(active bool) [][4]float64 {
	segments := [][4]float64{
		{6, 12, 23, 12}, {7, 12, 8, 23}, {8, 23, 11, 26},
		{11, 26, 19, 26}, {19, 26, 22, 23}, {22, 23, 23, 12},
		{5, 29, 25, 29}, {23, 14, 27, 14}, {27, 14, 29, 17},
		{29, 17, 28, 21}, {28, 21, 23, 22},
	}
	if active {
		segments = append(segments, [4]float64{10, 8, 11, 5}, [4]float64{11, 5, 10, 2},
			[4]float64{16, 8, 17, 5}, [4]float64{17, 5, 16, 2},
			[4]float64{22, 8, 23, 5}, [4]float64{23, 5, 22, 2})
	}
	return segments
}

func cupContains(x, y float64, segments [][4]float64) bool {
	for _, segment := range segments {
		dx, dy := segment[2]-segment[0], segment[3]-segment[1]
		position := math.Max(0, math.Min(1, ((x-segment[0])*dx+(y-segment[1])*dy)/(dx*dx+dy*dy)))
		if math.Hypot(x-segment[0]-position*dx, y-segment[1]-position*dy) <= 1.1 {
			return true
		}
	}
	return false
}
