package drawer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"live-semantic/src/domain/model"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

const (
	defaultBoxThickness  = 5
	defaultLabelFontSize = 12
)

type BoxDrawer struct {
	img           *image.RGBA
	boxes         []model.BoundingBox
	boxThickness  int
	labelFont     font.Face
	labelFontSize float64
}

// NewBoxDrawer creates a new BoxDrawer instance with the given image.
func NewBoxDrawer(img image.Image, boxes []model.BoundingBox) *BoxDrawer {
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
		boxThickness:  defaultBoxThickness,
		labelFont:     defaultFontFace,
		labelFontSize: defaultLabelFontSize,
	}
}

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

func (bd *BoxDrawer) SetBoxThickness(thickness int) {
	if thickness <= 0 {
		fmt.Println("Invalid box thickness, must be greater than 0")
		return
	}
	bd.boxThickness = thickness
}

// ToImage returns the current image.
func (bd *BoxDrawer) ToImage() *image.RGBA {
	return bd.img
}

// ToBytes returns the current image.
func (bd *BoxDrawer) ToBytes() []byte {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, bd.img, nil); err != nil {
		return nil
	}
	return buf.Bytes()
}

// DrawBox draws bounding boxes on the image with labels and confidence scores.
func (bd *BoxDrawer) Draw() {
	for _, box := range bd.boxes {
		label := fmt.Sprintf("%s %.2f", box.Label, box.Confidence)
		rect := box.ToRect()
		bd.DrawRect(bd.img, rect, model.ClassLabelColor(box.Label), bd.boxThickness)
		bd.WriteLabel(bd.img, rect.Min.X+10, rect.Min.Y+30, label)
	}
}

// DrawRect draws a rectangle on the image with the specified color and thickness
func (bd *BoxDrawer) DrawRect(img *image.RGBA, rect image.Rectangle, col color.Color, thickness int) {
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

// DrawLabel draws a label on the image at the specified position
func (bd *BoxDrawer) WriteLabel(img *image.RGBA, x, y int, label string) {
	col := model.ClassLabelColor(label)
	point := fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: bd.labelFont,
		Dot:  point,
	}
	d.DrawString(label)
}
