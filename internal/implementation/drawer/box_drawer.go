package drawer

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	defaultLabelFontSize = 12
)

// BoxDrawer composites a set of Box values onto a copy of img, converted
// to *image.RGBA for in-place pixel mutation.
type BoxDrawer struct {
	img           *image.RGBA
	boxes         []Box
	labelFont     font.Face
	labelFontSize float64
}

// NewBoxDrawer creates a new BoxDrawer for img and boxes, loading the
// default label font — returns nil if img is nil or the default font
// fails to load (callers should treat a nil result as "skip drawing,
// render the original frame").
func NewBoxDrawer(img image.Image, boxes []Box) *BoxDrawer {
	if img == nil {
		return nil
	}

	// Always convert to RGBA for mutability
	rgbaImg, ok := img.(*image.RGBA)
	if !ok {
		rgbaImg = image.NewRGBA(img.Bounds())
		draw.Draw(rgbaImg, img.Bounds(), img, image.Point{}, draw.Src)
	}

	fl := &FontLoader{}
	defaultFontFace, err := fl.LoadFont()
	if err != nil {
		fmt.Println("Error loading font:", err)
		return nil
	}

	return &BoxDrawer{
		img:           rgbaImg,
		boxes:         boxes,
		labelFont:     defaultFontFace,
		labelFontSize: defaultLabelFontSize,
	}
}

// SetFonts overrides the label font/size after construction — no-op on
// the current font if the new one fails to load.
func (bd *BoxDrawer) SetFonts(labelFontPath string, labelFontSize float64) {
	fl := &FontLoader{
		fontFace: labelFontPath,
		fontSize: labelFontSize,
	}
	var err error
	bd.labelFont, err = fl.LoadFont()
	if err != nil {
		fmt.Println("Error loading font:", err)
		return
	}
	bd.labelFontSize = labelFontSize
}

// ToImage returns the current (possibly already-drawn-on) image.
func (bd *BoxDrawer) ToImage() *image.RGBA {
	return bd.img
}

// ToBytes JPEG-encodes the current image, or nil on encoding failure.
func (bd *BoxDrawer) ToBytes() []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, bd.img, nil); err != nil {
		return nil
	}
	return buf.Bytes()
}

// Draw renders every box (rectangle + label) onto bd's image, in order.
func (bd *BoxDrawer) Draw() {
	for _, box := range bd.boxes {
		bd.DrawRect(bd.img, box)
		bd.WriteLabel(bd.img, box, 10, 30)
	}
}

// DrawRect draws box's outline onto img, one pixel ring per unit of
// thickness (a hollow rectangle, not filled).
func (bd *BoxDrawer) DrawRect(img *image.RGBA, box Box) {
	col := box.Color
	thickness := box.Thickness
	rect := box.ToRect()

	for t := 0; t < thickness; t++ {
		// Top
		for x := rect.Min.X + t; x < rect.Max.X-t; x++ {
			img.Set(x, rect.Min.Y+t, col)
		}
		// Bottom
		for x := rect.Min.X + t; x < rect.Max.X-t; x++ {
			img.Set(x, rect.Max.Y-1-t, col)
		}
		// Left
		for y := rect.Min.Y + t; y < rect.Max.Y-t; y++ {
			img.Set(rect.Min.X+t, y, col)
		}
		// Right
		for y := rect.Min.Y + t; y < rect.Max.Y-t; y++ {
			img.Set(rect.Max.X-1-t, y, col)
		}
	}
}

// WriteLabel draws box.Description onto img, offset (x, y) pixels from
// box's top-left corner.
func (bd *BoxDrawer) WriteLabel(img *image.RGBA, box Box, x, y int) {
	rect := box.ToRect()
	col := box.Color
	point := fixed.Point26_6{X: fixed.I(rect.Min.X + x), Y: fixed.I(rect.Min.Y + y)}

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: bd.labelFont,
		Dot:  point,
	}

	d.DrawString(box.Description)
}
