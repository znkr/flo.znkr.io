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
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"flo.znkr.io/generator/site"
)

// Width and Height are the size a card is drawn at, which a link preview
// states next to the image URL.
const (
	Width, Height = 1200, 630
)

// The card is laid out like a page: the gradient as a thin line at the top,
// the site name below it, and the title, centered on the card from top to
// bottom and set on the left margin.
const (
	margin       = 80
	barHeight    = 6
	nameTop      = 56
	nameSize     = 32
	titleSize    = 76
	minTitleSize = 48
	maxLines     = 4
	siteName     = "flo.znkr.io"
)

// The colors are the site's --color-bg and --color-text.
var (
	bg   = color.RGBA{0xff, 0xff, 0xff, 0xff}
	text = color.RGBA{0x1f, 0x21, 0x29, 0xff}

	// The header gradient, left to right.
	gradient = []struct {
		at float64
		c  color.RGBA
	}{
		{0, color.RGBA{0x22, 0xe1, 0xff, 0xff}},
		{0.48, color.RGBA{0x1d, 0x8f, 0xe1, 0xff}},
		{1, color.RGBA{0x74, 0x6f, 0xd2, 0xff}},
	}

	bold = mustParse(gobold.TTF)
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

	nameFace, err := newFace(bold, nameSize)
	if err != nil {
		return nil, err
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(text), Face: nameFace}
	d.Dot = fixed.P(margin, nameTop+nameFace.Metrics().Ascent.Ceil())
	d.DrawString(siteName)

	// The title, at the largest size that fits the lines allowed.
	size := titleSize
	var titleFace font.Face
	var title []string
	for {
		titleFace, err = newFace(bold, size)
		if err != nil {
			return nil, err
		}
		title = wrap(titleFace, m.Title, Width-2*margin)
		if len(title) <= maxLines || size <= minTitleSize {
			break
		}
		size -= 8
	}
	titleLeading := size * 118 / 100

	drawLines(img, titleFace, text, title, (Height-len(title)*titleLeading)/2, titleLeading)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func newFace(f *opentype.Font, size int) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: float64(size), DPI: 72, Hinting: font.HintingNone})
}

// drawLines draws lines on the left margin, each in a box leading pixels
// high starting at top.
func drawLines(img *image.RGBA, face font.Face, c color.Color, lines []string, top, leading int) {
	met := face.Metrics()
	// The baseline that centers the font's ascent and descent in the box.
	offset := (leading-(met.Ascent+met.Descent).Ceil())/2 + met.Ascent.Ceil()
	d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face}
	for _, line := range lines {
		d.Dot = fixed.P(margin, top+offset)
		d.DrawString(line)
		top += leading
	}
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
