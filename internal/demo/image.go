package demo

import (
	"bytes"
	"image"
	"image/color"
	imagepng "image/png"
)

func png(kind string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 360, 240))
	switch kind {
	case "store":
		drawStore(img)
	default:
		drawOnboarding(img)
	}
	var buf bytes.Buffer
	_ = imagepng.Encode(&buf, img)
	return buf.Bytes()
}

func drawOnboarding(img *image.RGBA) {
	const w, h = 360, 240
	from := color.RGBA{0x2f, 0xc6, 0xf6, 0xff}
	to := color.RGBA{0x39, 0xd3, 0xb6, 0xff}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := float64(x+y) / float64(w+h)
			img.SetRGBA(x, y, mix(from, to, t))
		}
	}
	white := color.RGBA{0xfa, 0xfd, 0xff, 0xff}
	dots := []color.RGBA{
		{0x2f, 0xc6, 0xf6, 0xff},
		{0xff, 0xa9, 0x00, 0xff},
		{0x7b, 0xd5, 0x00, 0xff},
	}
	grey := color.RGBA{0xb9, 0xc7, 0xce, 0xff}
	for i := 0; i < 3; i++ {
		x0 := 24 + i*108
		y0, x1, y1 := 36, x0+88, 204
		fillRoundRect(img, x0, y0, x1, y1, 14, white)
		cx, cy := x0+44, y0+52
		fillCircle(img, cx, cy, 22, dots[i])
		fillRoundRect(img, x0+16, y0+96, x1-16, y0+108, 6, grey)
		fillRoundRect(img, x0+16, y0+116, x0+58, y0+128, 6, grey)
		fillRoundRect(img, x0+16, y0+136, x1-28, y0+148, 6, grey)
	}
}

func drawStore(img *image.RGBA) {
	const w, h = 360, 240
	bg := color.RGBA{0x0d, 0x12, 0x15, 0xff}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, bg)
		}
	}
	fillRoundRect(img, 24, 20, 336, 58, 12, color.RGBA{0xfa, 0xfd, 0xff, 0xff})
	teal := color.RGBA{0x39, 0xd3, 0xb6, 0xff}
	blue := color.RGBA{0x2f, 0xc6, 0xf6, 0xff}
	heights := []int{72, 112, 92, 144, 120, 168}
	const base = 214
	for i, bh := range heights {
		x0 := 42 + i*52
		c := teal
		if i%2 == 1 {
			c = blue
		}
		fillRoundRect(img, x0, base-bh, x0+28, base, 6, c)
	}
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		0xff,
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func fillRoundRect(img *image.RGBA, x0, y0, x1, y1, r int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			dx := 0
			if x < x0+r {
				dx = x0 + r - x
			} else if x > x1-r {
				dx = x - (x1 - r)
			}
			dy := 0
			if y < y0+r {
				dy = y0 + r - y
			} else if y > y1-r {
				dy = y - (y1 - r)
			}
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}
