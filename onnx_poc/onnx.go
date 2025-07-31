package onnx_poc

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math/rand"
	"os"
	"runtime"
	"sort"

	"image/color"
	"image/draw"
	"image/png"
	"path/filepath"

	"github.com/8ff/prettyTimer"
	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Path to the ONNX model file
const (
	// modelPath is the path to the YOLOv8s ONNX model
	basePath = "./"
)

var modelPath = basePath + "models/yolo11s.onnx"
var imagePath = basePath + "bus.jpg"
var useCoreML = false

// Array of YOLOv8 class labels
// This is the list of classes that the YOLOv8 model can detect.
// It is used to map the class IDs to human-readable labels.
// The order of the classes must match the order in the YOLOv8 model.
// The classes are based on the COCO dataset, which is a common dataset for object detection
var yoloClasses = []string{
	"person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat",
	"traffic light", "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat", "dog", "horse",
	"sheep", "cow", "elephant", "bear", "zebra", "giraffe", "backpack", "umbrella", "handbag", "tie",
	"suitcase", "frisbee", "skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove",
	"skateboard", "surfboard", "tennis racket", "bottle", "wine glass", "cup", "fork", "knife", "spoon",
	"bowl", "banana", "apple", "sandwich", "orange", "broccoli", "carrot", "hot dog", "pizza", "donut",
	"cake", "chair", "couch", "potted plant", "bed", "dining table", "toilet", "tv", "laptop", "mouse",
	"remote", "keyboard", "cell phone", "microwave", "oven", "toaster", "sink", "refrigerator", "book",
	"clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush",
}

type ModelSession struct {
	Session *ort.AdvancedSession
	Input   *ort.Tensor[float32]
	Output  *ort.Tensor[float32]
}

type boundingBox struct {
	label          string
	confidence     float32
	x1, y1, x2, y2 float32
}

func Run() int {

	if os.Getenv("USE_COREML") == "true" {
		useCoreML = true
	}

	// Read the input image into a image.Image object
	pic, e := loadImageFile(imagePath)
	if e != nil {
		fmt.Printf("Error loading input image: %s\n", e)
		return 1
	}
	originalWidth := pic.Bounds().Canon().Dx()
	originalHeight := pic.Bounds().Canon().Dy()

	// Initialize the model session
	modelSession, e := initSession()
	if e != nil {
		fmt.Printf("Error creating session and tensors: %s\n", e)
		return 1
	}
	defer modelSession.Destroy()

	timingStats := prettyTimer.NewTimingStats()
	// Run the detection 5 times
	var boxes []boundingBox
	for i := 0; i < 1; i++ {
		e = prepareInput(pic, modelSession.Input)
		if e != nil {
			fmt.Printf("Error converting image to network input: %s\n", e)
			return 1
		}

		timingStats.Start()
		e = modelSession.Session.Run()
		if e != nil {
			fmt.Printf("Error running ORT session: %s\n", e)
			return 1
		}
		timingStats.Finish()

		// Print the results
		boxes = processOutput(modelSession.Output.GetData(), originalWidth,
			originalHeight)
		for i, box := range boxes {
			fmt.Printf("Box %d: %s\n", i, &box)
		}
	}
	timingStats.PrintStats()

	// Draw the bounding boxes on the original image
	fontFace, err := LoadFontFace(basePath+"models/Roboto-Regular.ttf", 30.0)
	if err != nil {
		fmt.Printf("Error loading font: %s\n", err)
		return 1
	}

	SaveImageWithBoxes(pic, boxes, imagePath, "output_", 8, fontFace)

	return 0
}

// loadImageFile loads an image from the given file path and returns it as an image.Image.
// It supports JPEG, PNG, and GIF formats.
// Returns an error if the file cannot be opened or decoded.
func loadImageFile(filePath string) (image.Image, error) {
	f, e := os.Open(filePath)
	if e != nil {
		return nil, fmt.Errorf("Error opening %s: %w", filePath, e)
	}
	defer f.Close()
	pic, _, e := image.Decode(f)
	if e != nil {
		return nil, fmt.Errorf("Error decoding %s: %w", filePath, e)
	}
	return pic, nil
}

// Populates a yolov8n input tensor with the contents of the given image.
// The input tensor is expected to be of shape [1, 3, 640, 640].
// The image is resized to 640x640 using Lanczos3 algorithm and the pixel values
// are normalized to the range [0, 1].
// The red, green, and blue channels are stored in the first, second, and third
// channels of the tensor respectively.
// Returns an error if the input tensor is not of the expected shape or if the
// image is not in the expected format.
func prepareInput(pic image.Image, dst *ort.Tensor[float32]) error {
	data := dst.GetData()
	channelSize := 640 * 640
	if len(data) < (channelSize * 3) {
		return fmt.Errorf("Destination tensor only holds %d floats, needs "+
			"%d (make sure it's the right shape!)", len(data), channelSize*3)
	}
	redChannel := data[0:channelSize]
	greenChannel := data[channelSize : channelSize*2]
	blueChannel := data[channelSize*2 : channelSize*3]

	// Resize the image to 640x640 using Lanczos3 algorithm
	pic = resize.Resize(640, 640, pic, resize.Lanczos3)
	i := 0
	for y := 0; y < 640; y++ {
		for x := 0; x < 640; x++ {
			r, g, b, _ := pic.At(x, y).RGBA()
			redChannel[i] = float32(r>>8) / 255.0
			greenChannel[i] = float32(g>>8) / 255.0
			blueChannel[i] = float32(b>>8) / 255.0
			i++
		}
	}

	return nil
}

// Returns the path to the shared library based on the current OS and architecture.
func getSharedLibPath() string {
	fmt.Printf("OS and architecture: %s %s\n", runtime.GOOS, runtime.GOARCH)

	if runtime.GOOS == "windows" {
		if runtime.GOARCH == "amd64" {
			return basePath + "./runtime/win/onnxruntime.dll"
		}
	}
	if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			return basePath + "./runtime/osx/onnxruntime_arm64.dylib"
		}
		if runtime.GOARCH == "amd64" {
			return basePath + "./runtime/osx/onnxruntime_amd64.dylib"
		}

	}
	if runtime.GOOS == "linux" {
		if runtime.GOARCH == "arm64" {
			return basePath + "./runtime/linux/onnxruntime_arm64.so"
		}
		return basePath + "./runtime/linux/onnxruntime.so"
	}
	panic("Unable to find a version of the onnxruntime library supporting this system.")
}

// Initializes the ONNX session and input/output tensors.
// Returns a ModelSession containing the session and tensors, or an error if initialization fails.
func initSession() (*ModelSession, error) {
	ort.SetSharedLibraryPath(getSharedLibPath())
	err := ort.InitializeEnvironment()
	if err != nil {
		return nil, fmt.Errorf("Error initializing ORT environment: %w", err)
	}

	inputShape := ort.NewShape(1, 3, 640, 640)
	inputTensor, err := ort.NewEmptyTensor[float32](inputShape)
	if err != nil {
		return nil, fmt.Errorf("Error creating input tensor: %w", err)
	}
	outputShape := ort.NewShape(1, 84, 8400)
	outputTensor, err := ort.NewEmptyTensor[float32](outputShape)
	if err != nil {
		inputTensor.Destroy()
		return nil, fmt.Errorf("Error creating output tensor: %w", err)
	}
	options, err := ort.NewSessionOptions()
	if err != nil {
		inputTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("Error creating ORT session options: %w", err)
	}
	defer options.Destroy()

	// If CoreML is enabled, append the CoreML execution provider
	if useCoreML {
		err = options.AppendExecutionProviderCoreML(0)
		if err != nil {
			inputTensor.Destroy()
			outputTensor.Destroy()
			return nil, fmt.Errorf("Error enabling CoreML: %w", err)
		}
	}

	session, err := ort.NewAdvancedSession(modelPath,
		[]string{"images"}, []string{"output0"},
		[]ort.ArbitraryTensor{inputTensor},
		[]ort.ArbitraryTensor{outputTensor},
		options)

	if err != nil {
		inputTensor.Destroy()
		outputTensor.Destroy()
		return nil, fmt.Errorf("Error creating ORT session: %w", err)
	}

	return &ModelSession{
		Session: session,
		Input:   inputTensor,
		Output:  outputTensor,
	}, nil
}

// Destroys the session and input/output tensors to free resources.
// This should be called when the session is no longer needed.
// It is safe to call this method multiple times.
// It is also safe to call this method even if the session was not successfully created.
func (m *ModelSession) Destroy() {
	m.Session.Destroy()
	m.Input.Destroy()
	m.Output.Destroy()
}

// String returns a string representation of the bounding box, including its label and coordinates.
// This is useful for debugging and logging purposes.
func (b *boundingBox) String() string {
	return fmt.Sprintf("Object %s (confidence %f): (%f, %f), (%f, %f)",
		b.label, b.confidence, b.x1, b.y1, b.x2, b.y2)
}

// This loses precision, but recall that the boundingBox has already been
// scaled up to the original image's dimensions. So, it will only lose
// fractional pixels around the edges.
func (b *boundingBox) toRect() image.Rectangle {
	return image.Rect(int(b.x1), int(b.y1), int(b.x2), int(b.y2)).Canon()
}

// Returns the area of b in pixels, after converting to an image.Rectangle.
func (b *boundingBox) rectArea() int {
	size := b.toRect().Size()
	return size.X * size.Y
}

// Returns the intersection area of this bounding box with another bounding box.
// This is calculated by finding the intersection rectangle and returning its area.
// If the rectangles do not intersect, this will return 0.
// This is useful for determining how much two bounding boxes overlap.
func (b *boundingBox) intersection(other *boundingBox) float32 {
	r1 := b.toRect()
	r2 := other.toRect()
	intersected := r1.Intersect(r2).Canon().Size()
	return float32(intersected.X * intersected.Y)
}

// Returns the union area of this bounding box with another bounding box.
// This is calculated by adding the areas of both rectangles and subtracting the intersection area.
// This is useful for determining how much two bounding boxes overlap in total.
func (b *boundingBox) union(other *boundingBox) float32 {
	intersectArea := b.intersection(other)
	totalArea := float32(b.rectArea() + other.rectArea())
	return totalArea - intersectArea
}

// Returns the Intersection over Union (IoU) of this bounding box with another bounding box.
// This is calculated as the intersection area divided by the union area.
// The IoU is a measure of how much two bounding boxes overlap.
// It ranges from 0 (no overlap) to 1 (perfect overlap).
// This is useful for determining how similar two bounding boxes are.
// This won't be entirely precise due to conversion to the integral rectangles
// from the image.Image library, but we're only using it to estimate which
// boxes are overlapping too much, so some imprecision should be OK.
func (b *boundingBox) iou(other *boundingBox) float32 {
	return b.intersection(other) / b.union(other)
}

// processOutput processes the output of the YOLOv8 model and returns a slice of bounding boxes.
// It iterates through the output array, finds the class with the highest probability for each index
func processOutput(output []float32, originalWidth,
	originalHeight int) []boundingBox {
	boundingBoxes := make([]boundingBox, 0, 8400)

	var classID int
	var probability float32

	// Iterate through the output array, considering 8400 indices
	for idx := 0; idx < 8400; idx++ {
		// Iterate through 80 classes and find the class with the highest probability
		probability = -1e9
		for col := 0; col < 80; col++ {
			currentProb := output[8400*(col+4)+idx]
			if currentProb > probability {
				probability = currentProb
				classID = col
			}
		}

		// If the probability is less than 0.5, continue to the next index
		if probability < 0.5 {
			continue
		}

		// Extract the coordinates and dimensions of the bounding box
		xc, yc := output[idx], output[8400+idx]
		w, h := output[2*8400+idx], output[3*8400+idx]
		x1 := (xc - w/2) / 640 * float32(originalWidth)
		y1 := (yc - h/2) / 640 * float32(originalHeight)
		x2 := (xc + w/2) / 640 * float32(originalWidth)
		y2 := (yc + h/2) / 640 * float32(originalHeight)

		// Append the bounding box to the result
		boundingBoxes = append(boundingBoxes, boundingBox{
			label:      yoloClasses[classID],
			confidence: probability,
			x1:         x1,
			y1:         y1,
			x2:         x2,
			y2:         y2,
		})
	}

	// Sort the bounding boxes by probability
	sort.Slice(boundingBoxes, func(i, j int) bool {
		return boundingBoxes[i].confidence < boundingBoxes[j].confidence
	})

	// Define a slice to hold the final result
	mergedResults := make([]boundingBox, 0, len(boundingBoxes))

	// Iterate through sorted bounding boxes, removing overlaps
	for _, candidateBox := range boundingBoxes {
		overlapsExistingBox := false
		for _, existingBox := range mergedResults {
			if (&candidateBox).iou(&existingBox) > 0.7 {
				overlapsExistingBox = true
				break
			}
		}
		if !overlapsExistingBox {
			mergedResults = append(mergedResults, candidateBox)
		}
	}

	// This will still be in sorted order by confidence
	return mergedResults
}

// DrawBoundingBoxes dessine les rectangles et labels sur l'image pour chaque bounding box.
// LoadFontFace loads a TTF font from disk and returns a font.Face with the desired size.
func LoadFontFace(fontPath string, size float64) (font.Face, error) {
	fontFile, err := os.Open(fontPath)
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
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	return face, err
}

// DrawBoundingBoxes dessine les rectangles et labels sur l'image pour chaque bounding box.
// Cette fonction prend en entrée une image, une liste de bounding boxes, une épaisseur de rectangle et une police pour les labels.
// Elle retourne une nouvelle image avec les rectangles et labels dessinés.
// img: image d'entrée, boxes: liste des bounding boxes,
// thickness: épaisseur du rectangle, fontFace: police pour le label
func DrawBoundingBoxes(img image.Image, boxes []boundingBox, thickness int, fontFace font.Face) image.Image {
	out := image.NewRGBA(img.Bounds())
	draw.Draw(out, img.Bounds(), img, image.Point{}, draw.Src)

	labelColor := map[string]color.Color{}
	for _, box := range boxes {
		if _, exists := labelColor[box.label]; !exists {
			labelColor[box.label] = color.RGBA{uint8(rand.Intn(256)), uint8(rand.Intn(256)), uint8(rand.Intn(256)), 255}
		}
		rect := box.toRect()
		drawRect(out, rect, labelColor[box.label], thickness)

		label := fmt.Sprintf("%s %.2f", box.label, box.confidence)
		drawLabel(out, rect.Min.X, rect.Min.Y-10, label, fontFace)
	}

	return out
}

// drawRect dessine un rectangle avec une épaisseur donnée.
// Cette fonction prend en entrée une image, un rectangle, une couleur et une épaisseur.
// Elle dessine le rectangle sur l'image en utilisant la couleur spécifiée et l'épaisseur.
// img: image sur laquelle dessiner, rect: rectangle à dessiner,
// col: couleur du rectangle, thickness: épaisseur du rectangle
// Cette fonction dessine le rectangle en traçant les bords supérieur, inférieur, gauche et
// droit du rectangle avec l'épaisseur spécifiée.
// Elle est utilisée pour dessiner les bounding boxes sur l'image.
func drawRect(img *image.RGBA, rect image.Rectangle, col color.Color, thickness int) {
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

// drawLabel dessine un texte avec la police donnée.
// Cette fonction prend en entrée une image, les coordonnées x et y du point de départ,
// le texte à dessiner et la police à utiliser.
// Elle utilise la police pour dessiner le texte sur l'image à la position spécifiée.
// img: image sur laquelle dessiner, x: coordonnée x du point de départ,
// y: coordonnée y du point de départ, label: texte à dessiner, face: police à utiliser
// Cette fonction utilise la police pour dessiner le texte sur l'image à la position spécifiée.
// Elle crée un objet Drawer pour dessiner le texte avec la couleur jaune sur l'image.
// Le point de départ est spécifié par les coordonnées x et y.
// Elle est utilisée pour dessiner les labels des bounding boxes sur l'image.
func drawLabel(img *image.RGBA, x, y int, label string, face font.Face) {
	col := color.RGBA{255, 255, 0, 255} // Jaune
	point := fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  point,
	}
	d.DrawString(label)
}

// SaveImageWithBoxes dessine les bounding boxes et labels puis sauvegarde l'image avec un préfixe.
// Cette fonction prend en entrée une image, une liste de bounding boxes, le chemin original de l'image,
// un préfixe pour le nom du fichier, une épaisseur de rectangle et une police.
// Elle dessine les bounding boxes et labels sur l'image, puis sauvegarde l'image modifiée
// dans le même répertoire que l'image originale avec le préfixe ajouté au nom du fichier.
// img: image d'entrée, boxes: liste des bounding boxes,
// originalPath: chemin original de l'image, prefix: préfixe pour le nom du fichier,
// thickness: épaisseur du rectangle, fontFace: police pour les labels
// Cette fonction utilise la fonction DrawBoundingBoxes pour dessiner les bounding boxes et labels
func SaveImageWithBoxes(img image.Image, boxes []boundingBox, originalPath string, prefix string, thickness int, fontFace font.Face) error {
	out := DrawBoundingBoxes(img, boxes, thickness, fontFace)
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	newName := prefix + name + ext
	newPath := filepath.Join(dir, newName)

	outFile, err := os.Create(newPath)
	if err != nil {
		return err
	}
	defer outFile.Close()
	return png.Encode(outFile, out)
}
