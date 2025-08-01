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
	camera      *gocv.VideoCapture
	frameNumber uint64
}

// NewMacOsCameraSource creates a new MacOsCameraSource instance with the given device ID.
func NewMacOsCameraSource() (*MacOsCameraSource, error) {
	return &MacOsCameraSource{}, nil
}

// Start implements the VideoHandler.Start.
func (s *MacOsCameraSource) Start() error {

	deviceID := listDevices()

	if deviceID == -1 {
		return domain.ErrNoCameraFound
	}

	webcam, err := gocv.OpenVideoCapture(deviceID)
	if err != nil {
		return domain.ErrCouldNotOpenCamera
	}

	s.camera = webcam

	if s.camera == nil {
		return domain.ErrCameraNotInitialized
	}

	return nil
}

// NextFrame implements the VideoHandler.NextFrame.
func (s *MacOsCameraSource) NextFrame() (*model.Frame, error) {
	img := gocv.NewMat()
	if ok := s.camera.Read(&img); !ok || img.Empty() {
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

// Close implements the VideoHandler.Close.
func (s *MacOsCameraSource) Close() error {
	return s.camera.Close()
}

func listDevices() int {
	for i := 0; i < 10; i++ {
		camera, err := gocv.OpenVideoCapture(i)
		if err == nil && camera.IsOpened() {
			fmt.Printf("Camera found at deviceID: %d\n", i)
			camera.Close()
			return i
		}
	}
	fmt.Println("No camera found.")

	return -1
}
