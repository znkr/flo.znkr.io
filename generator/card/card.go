// Package card draws the image a link to a page is previewed with.
package card

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"flo.znkr.io/generator/site"
)

// Width and Height are the size a card is drawn at, which a link preview
// states next to the image URL.
const (
	Width, Height = 1200, 630
)

// The card is laid out like the site's header: the gradient as a bar at the
// top, the title, and the site name at the bottom with the mark in front of it.
const (
	margin       = 80
	barHeight    = 14
	titleTop     = 190
	titleSize    = 76
	minTitleSize = 48
	maxLines     = 4
	nameSize     = 40
	markSize     = 24
	siteName     = "flo.znkr.io"
)

var (
	bg       = color.RGBA{0xff, 0xff, 0xff, 0xff}
	text     = color.RGBA{0x16, 0x18, 0x1c, 0xff}
	textSoft = color.RGBA{0x47, 0x4d, 0x55, 0xff}

	// The header gradient, left to right.
	gradient = []struct {
		at float64
		c  color.RGBA
	}{
		{0, color.RGBA{0x22, 0xe1, 0xff, 0xff}},
		{0.48, color.RGBA{0x1d, 0x8f, 0xe1, 0xff}},
		{1, color.RGBA{0x74, 0x6f, 0xd2, 0xff}},
	}

	bold    = mustParse(gobold.TTF)
	regular = mustParse(goregular.TTF)
)

func mustParse(ttf []byte) *opentype.Font {
	f, err := opentype.Parse(ttf)
	if err != nil {
		panic(err)
	}
	return f
}

// Render returns the card for the page described by m, as a PNG.
func Render(m site.Metadata) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	fillGradient(img, image.Rect(0, 0, Width, barHeight))

	// The title, at the largest size that fits the lines allowed.
	size := titleSize
	var face font.Face
	var lines []string
	for {
		var err error
		face, err = opentype.NewFace(bold, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
		if err != nil {
			return nil, err
		}
		lines = wrap(face, m.Title, Width-2*margin)
		if len(lines) <= maxLines || size <= minTitleSize {
			break
		}
		size -= 8
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(text), Face: face}
	lineHeight := size * 118 / 100
	for i, line := range lines {
		d.Dot = fixed.P(margin, titleTop+i*lineHeight)
		d.DrawString(line)
	}

	// The site name, with the mark in front of it.
	face, err := opentype.NewFace(regular, &opentype.FaceOptions{Size: nameSize, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, err
	}
	baseline := Height - margin
	fillGradient(img, image.Rect(margin, baseline-markSize, margin+markSize, baseline))
	d = &font.Drawer{Dst: img, Src: image.NewUniform(textSoft), Face: face}
	d.Dot = fixed.P(margin+markSize+16, baseline)
	d.DrawString(siteName)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// wrap breaks s into lines no wider than maxWidth pixels in face, at spaces. A
// word wider than maxWidth is a line of its own.
func wrap(face font.Face, s string, maxWidth int) []string {
	var lines []string
	var line string
	for _, word := range strings.Fields(s) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if line != "" && font.MeasureString(face, candidate).Ceil() > maxWidth {
			lines = append(lines, line)
			line = word
			continue
		}
		line = candidate
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// fillGradient paints r with the gradient, running left to right across r.
func fillGradient(img *image.RGBA, r image.Rectangle) {
	for x := r.Min.X; x < r.Max.X; x++ {
		c := gradientAt(float64(x-r.Min.X) / float64(r.Dx()-1))
		for y := r.Min.Y; y < r.Max.Y; y++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func gradientAt(t float64) color.RGBA {
	for i := 1; i < len(gradient); i++ {
		a, b := gradient[i-1], gradient[i]
		if t > b.at {
			continue
		}
		f := (t - a.at) / (b.at - a.at)
		return color.RGBA{
			lerp(a.c.R, b.c.R, f),
			lerp(a.c.G, b.c.G, f),
			lerp(a.c.B, b.c.B, f),
			0xff,
		}
	}
	return gradient[len(gradient)-1].c
}

func lerp(a, b uint8, f float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*f + 0.5)
}
