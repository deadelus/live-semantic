package drawer

import (
	"io"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

const defaultFontPath = "assets/fonts/Roboto-Regular.ttf"

// FontLoader resolves and parses a TTF/OTF font into a font.Face usable
// by BoxDrawer, falling back to defaults for either field left unset.
type FontLoader struct {
	fontFace string
	fontSize float64
}

// LoadFont reads and parses fl.fontFace at fl.fontSize (defaulting to
// defaultFontPath / size 20 if unset), returning a ready-to-use font.Face.
func (fl *FontLoader) LoadFont() (font.Face, error) {
	if fl.fontFace == "" {
		fl.fontFace = defaultFontPath // Default font if not provided
	}

	if fl.fontSize <= 0 {
		fl.fontSize = 20 // Default font size if not specified
	}

	fontFile, err := os.Open(fl.fontFace)
	if err != nil {
		return nil, err
	}
	defer fontFile.Close()
	fontBytes, err := io.ReadAll(fontFile)
	if err != nil {
		return nil, err
	}
	fnt, err := opentype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(fnt, &opentype.FaceOptions{
		Size:    fl.fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	return face, err
}
