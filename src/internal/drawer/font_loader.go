package drawer

import (
	"io"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

const defaultFontPath = "src/assets/fonts/Roboto-Regular.ttf"

type FontLoader struct {
	fontFace string
	fontSize float64
}

// loadFontFace loads the font face for drawing text on images.
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
