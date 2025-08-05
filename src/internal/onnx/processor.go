package onnx

import (
	"fmt"
	"image"
	"sort"

	"github.com/nfnt/resize"
	ort "github.com/yalue/onnxruntime_go"
)

type Processor struct {
	// Image is the image to be processed.
	Image image.Image
	// Model classes is the list of classes that the model can detect.
	// This is used to map the class ID to the class name.
	// It should match the order of classes in the model.
	// For YOLOv11s, this is a list of 80 classes.
	ModelClasses []string
	// ModelHeight and ModelWidth are the dimensions to which input images are resized.
	ModelHeight uint
	ModelWidth  uint
	// ModelInputChannels is the number of channels in the input image (3 for RGB).
	ModelInputChannels uint
	// ModelOutputClasses is the number of classes the model can detect.
	ModelOutputClasses uint
	// ModelDetections is the number of detections the model can output.
	ModelDetections uint
	// ThresholdConfidence is the minimum confidence threshold for detections.
	ThresholdConfidence float32
}

// ProcessInput prepares the input frame for the model.
// It resizes the image to the model's expected input size and normalizes the pixel values.
func (p *Processor) Input(tensor *ort.Tensor[float32]) error {
	data := tensor.GetData()
	channelSize := p.ModelHeight * p.ModelWidth
	if len(data) < int(channelSize*p.ModelInputChannels) {
		return fmt.Errorf("destination tensor only holds %d floats, needs %d (make sure it's the right shape!)", len(data), channelSize*p.ModelInputChannels)
	}
	redChannel := data[0:channelSize]
	greenChannel := data[channelSize : channelSize*2]
	blueChannel := data[channelSize*2 : channelSize*3]

	originalBounds := p.Image.Bounds()
	originalWidth := originalBounds.Dx()
	originalHeight := originalBounds.Dy()

	p.Image = resize.Resize(p.ModelWidth, p.ModelHeight, p.Image, resize.Lanczos3)

	i := 0
	for y := 0; y < int(p.ModelHeight); y++ {
		for x := 0; x < int(p.ModelWidth); x++ {
			r, g, b, _ := p.Image.At(x, y).RGBA()
			redChannel[i] = float32(r>>8) / 255.0
			greenChannel[i] = float32(g>>8) / 255.0
			blueChannel[i] = float32(b>>8) / 255.0
			i++
		}
	}

	// Resize output image back to original dimensions and overwrite frame.Image
	p.Image = resize.Resize(uint(originalWidth), uint(originalHeight), p.Image, resize.Lanczos3)

	return nil
}

// PostProcessOutput processes the output of the YOLOv8 model and returns a slice of bounding boxes.
// It iterates through the output array, finds the class with the highest probability for each index
func (p *Processor) Output(tensor *ort.Tensor[float32]) []BoundingBox {
	output := tensor.GetData()
	if len(output) < int(p.ModelDetections*p.ModelOutputClasses) {
		fmt.Printf("Output tensor does not have enough data for %d detections with %d classes", p.ModelDetections, p.ModelOutputClasses)
		return nil
	}

	// Initialize a slice to hold the bounding boxes
	// ModelDetections is the number of detections, ModelOutputClasses is the number of classes + 4 coordinates
	// Each detection has 4 coordinates and a class probability for each class
	boundingBoxes := make([]BoundingBox, 0, p.ModelDetections)

	var classID int
	var probability float32

	// Iterate through the output array, considering ModelDetections indices
	for idx := 0; idx < int(p.ModelDetections); idx++ {
		// Iterate through ModelOutputClasses classes and find the class with the highest probability
		probability = -1e9
		for col := 0; col < int(p.ModelOutputClasses); col++ {
			currentProb := output[(int(p.ModelDetections)*(col+4))+idx]
			if currentProb > probability {
				probability = currentProb
				classID = col
			}
		}

		// If the probability is less than ThresholdConfidence, continue to the next index
		if probability < p.ThresholdConfidence {
			continue
		}

		// Extract the coordinates and dimensions of the bounding box
		// The coordinates are stored in the output tensor at the index for the current detection
		// The first two values are the center coordinates (xc, yc) and the next two are width (w) and height (h)
		// The output tensor is structured as follows:
		// [xc, yc, w, h, class1_prob, class2_prob, ..., class84_prob]
		xc, yc := output[idx], output[int(p.ModelDetections)+idx]
		w, h := output[2*int(p.ModelDetections)+idx], output[3*int(p.ModelDetections)+idx]

		// Calculate the bounding box coordinates in the original image dimensions
		// The coordinates are normalized to the model input size (Height x Width)
		// We scale them to the original image size
		// The bounding box is defined as (x1, y1, x2, y2)
		// where (x1, y1) is the top-left corner and (x2, y2) is the bottom-right corner
		x1 := (xc - w/2) / float32(p.ModelWidth) * float32(p.Image.Bounds().Max.X)
		y1 := (yc - h/2) / float32(p.ModelHeight) * float32(p.Image.Bounds().Max.Y)
		x2 := (xc + w/2) / float32(p.ModelWidth) * float32(p.Image.Bounds().Max.X)
		y2 := (yc + h/2) / float32(p.ModelHeight) * float32(p.Image.Bounds().Max.Y)

		// Append the bounding box to the result
		boundingBoxes = append(boundingBoxes, BoundingBox{
			Label:      p.ModelClasses[classID],
			Confidence: probability,
			X1:         x1,
			Y1:         y1,
			X2:         x2,
			Y2:         y2,
		})
	}

	// Sort the bounding boxes by probability
	sort.Slice(boundingBoxes, func(i, j int) bool {
		return boundingBoxes[i].Confidence < boundingBoxes[j].Confidence
	})

	// Define a slice to hold the final result
	mergedResults := make([]BoundingBox, 0, len(boundingBoxes))

	// Iterate through sorted bounding boxes, removing overlaps
	for _, candidateBox := range boundingBoxes {
		overlapsExistingBox := false
		for _, existingBox := range mergedResults {
			if (&candidateBox).IoU(&existingBox) > 0.7 {
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
