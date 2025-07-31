package macOsCamera

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"time"

	"live-semantic/src/domain"
	"live-semantic/src/domain/model"

	"gocv.io/x/gocv"
)

// MacOsCameraSource implements the VideoSource interface for macOS cameras.
type MacOsCameraSource struct {
	webcam      *gocv.VideoCapture
	frameNumber uint64
}

// NewMacOsCameraSource creates a new MacOsCameraSource instance with the given device ID.
func NewMacOsCameraSource() (*MacOsCameraSource, error) {
	deviceID := listDevices()

	if deviceID == -1 {
		return nil, nil
	}

	webcam, err := gocv.OpenVideoCapture(deviceID)
	if err != nil {
		return nil, domain.ErrCouldNotOpenCamera
	}

	return &MacOsCameraSource{webcam: webcam}, nil
}

// NextFrame implements the VideoSource interface for MacOsCameraSource.
func (s *MacOsCameraSource) NextFrame() (*model.Frame, error) {
	img := gocv.NewMat()
	if ok := s.webcam.Read(&img); !ok || img.Empty() {
		return nil, domain.ErrCouldNotReadFrameFromCamera
	}
	defer img.Close()

	buf := new(bytes.Buffer)
	imgData, err := img.ToImage()
	if err != nil {
		return nil, domain.ErrCouldNotConvertFrameToImage
	}
	err = jpeg.Encode(buf, imgData, nil)
	if err != nil {
		return nil, domain.ErrCouldNotEncodeFrameToJPEG
	}

	s.frameNumber++
	return &model.Frame{
		ImageData:   buf.Bytes(),
		ImageType:   model.ImageTypeJPEG,
		Width:       imgData.Bounds().Dx(),
		Height:      imgData.Bounds().Dy(),
		Timestamp:   time.Now(),
		FrameNumber: s.frameNumber,
	}, nil
}

// Close implements the VideoSource interface for MacOsCameraSource.
func (s *MacOsCameraSource) Close() error {
	return s.webcam.Close()
}

func listDevices() int {
	for i := 0; i < 10; i++ {
		webcam, err := gocv.OpenVideoCapture(i)
		if err == nil && webcam.IsOpened() {
			fmt.Printf("Camera found at deviceID: %d\n", i)
			webcam.Close()
			return i
		}
	}
	fmt.Println("No camera found.")

	return -1
}
