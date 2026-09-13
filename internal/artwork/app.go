package artwork

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

func AppPNG(size int) []byte {
	appImage := image.NewNRGBA(image.Rect(0, 0, size, size))
	espressoColor := color.NRGBA{R: 46, G: 33, B: 30, A: 255}
	porcelainColor := color.NRGBA{R: 255, G: 244, B: 230, A: 255}
	amberColor := color.NRGBA{R: 233, G: 170, B: 91, A: 255}
	for y := range size {
		for x := range size {
			dx := math.Max(math.Abs(float64(x)+0.5-float64(size)/2)-float64(size)*0.27, 0)
			dy := math.Max(math.Abs(float64(y)+0.5-float64(size)/2)-float64(size)*0.27, 0)
			if math.Hypot(dx, dy) <= float64(size)*0.18 {
				appImage.SetNRGBA(x, y, espressoColor)
			}
		}
	}
	cupSize := size * 3 / 5
	cupImage, _ := png.Decode(bytes.NewReader(CupPNG(cupSize, true)))
	for y := range cupSize {
		for x := range cupSize {
			_, _, _, alpha := cupImage.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}
			inkColor := porcelainColor
			if y < cupSize/3 {
				inkColor = amberColor
			}
			coverage := float64(alpha) / 65535
			inkColor.R = uint8(float64(inkColor.R)*coverage + float64(espressoColor.R)*(1-coverage))
			inkColor.G = uint8(float64(inkColor.G)*coverage + float64(espressoColor.G)*(1-coverage))
			inkColor.B = uint8(float64(inkColor.B)*coverage + float64(espressoColor.B)*(1-coverage))
			appImage.SetNRGBA(x+(size-cupSize)/2, y+(size-cupSize)/2, inkColor)
		}
	}
	var imageBuffer bytes.Buffer
	_ = png.Encode(&imageBuffer, appImage)
	return imageBuffer.Bytes()
}
