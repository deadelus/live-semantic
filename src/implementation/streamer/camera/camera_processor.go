package camera

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"live-semantic/src/domain"
	"live-semantic/src/domain/model"
	"time"

	"gocv.io/x/gocv"
)

// CameraProcessor implements the VideoSource interface for macOS cameras.
type CameraProcessor struct {
	camera         *gocv.VideoCapture
	window         *gocv.Window
	originalWindow *gocv.Window
	running        bool
}

// NewCameraProcessor creates a new CameraProcessor instance with the given device ID.
func NewCameraProcessor() *CameraProcessor {
	return &CameraProcessor{
		camera:         nil,
		window:         nil,
		originalWindow: nil,
		running:        false,
	}
}

// Initialize implements the StreamingProcessor interface for CameraProcessor
func (cp *CameraProcessor) Initialize() error {
	// Ouvrir la caméra (device 0 par défaut)
	var err error
	// Try using device 0 directly for compatibility with working GoCV projects
	cp.camera, err = gocv.OpenVideoCapture(0)
	if err != nil {
		return fmt.Errorf("OpenVideoCapture error: %w", err)
	}
	if !cp.camera.IsOpened() {
		return domain.ErrCouldNotOpenCamera
	}

	fmt.Println("Camera initialized successfully.")
	if !cp.camera.IsOpened() {
		fmt.Println("Warning: camera reports not opened after initialization.")
	}

	// Configurer la résolution (optionnel)
	cp.camera.Set(gocv.VideoCaptureFrameWidth, 640)
	cp.camera.Set(gocv.VideoCaptureFrameHeight, 480)

	// Créer les fenêtres d'affichage
	cp.originalWindow = gocv.NewWindow("Original")
	cp.window = gocv.NewWindow("AI Agent")

	// Positionner les fenêtres côte à côte
	cp.originalWindow.MoveWindow(100, 100)
	cp.window.MoveWindow(800, 100)

	return nil
}

// Start implements the StreamingProcessor interface for CameraProcessor
func (cp *CameraProcessor) Start(frameActionCallback func(*model.Frame) (*model.Frame, error)) error {
	if cp.camera == nil {
		return domain.ErrCameraNotInitialized
	}

	cp.running = true

	// Matrices pour stocker les frames
	imgMat := gocv.NewMat()
	defer imgMat.Close()
	defer cp.Cleanup()

	fmt.Println("Début du traitement vidéo...")
	fmt.Println("Appuyez sur 'q' pour quitter")

	frameCount := 0
	startTime := time.Now()

	for cp.running {
		// Lire une frame de la caméra
		ok := cp.camera.Read(&imgMat)
		if !ok {
			fmt.Println("Impossible de lire la frame de la caméra")
			break
		}

		if imgMat.Empty() {
			fmt.Println("Frame vide, arrêt du traitement")
			continue
		}

		// Call the frameActionCallback with the encoded image
		image, error := imgMat.ToImage()
		if error != nil {
			fmt.Printf("Erreur lors de la conversion de l'image: %v\n", error)
			continue
		}

		// Create a new frame with the image and current timestamp
		// Call the frameActionCallback with the new frame
		// Assuming frameActionCallback returns a processed frame
		// If it returns an error, we handle it accordingly
		outFrame, err := frameActionCallback(&model.Frame{
			Image:       image,
			Timestamp:   time.Now(),
			FrameNumber: uint64(frameCount),
		})

		if err != nil {
			fmt.Printf("Erreur lors du traitement de la frame: %v\n", err)
			continue
		}

		if outFrame == nil {
			fmt.Println("Frame traitée est nulle, arrêt du traitement")
			continue
		}

		if outFrame.Image == nil {
			fmt.Println("Image traitée est nulle, arrêt du traitement")
			continue
		}

		// Convert Image to Bytes for display
		buf := new(bytes.Buffer)
		err = jpeg.Encode(buf, outFrame.Image, &jpeg.Options{Quality: 90})
		if err != nil {
			return err
		}
		img := buf.Bytes()

		// Decode the JPEG image for display
		processedImgMat, err := gocv.IMDecode(img, gocv.IMReadColor)

		if err != nil || processedImgMat.Empty() {
			fmt.Printf("Erreur lors du décodage JPEG pour affichage: %v\n", err)
			continue
		}

		// Afficher les images
		cp.originalWindow.IMShow(imgMat)
		cp.window.IMShow(processedImgMat)
		processedImgMat.Close()

		// Gérer les événements clavier
		key := cp.window.WaitKey(1)
		if key >= 0 {
			switch key {
			case 'q', 'Q', 27: // 'q' ou Escape pour quitter
				cp.running = false
			}
		}

		// Calculer et afficher les FPS toutes les 30 frames
		frameCount++
		if frameCount%30 == 0 {
			elapsed := time.Since(startTime)
			fps := float64(frameCount) / elapsed.Seconds()
			fmt.Printf("FPS: %.1f\n", fps)
		}
	}

	return nil
}

// Stop implements the StreamingProcessor interface for CameraProcessor
func (cp *CameraProcessor) Stop() {
	cp.running = false
}

// Cleanup implements the StreamingProcessor interface for CameraProcessor
func (cp *CameraProcessor) Cleanup() {
	if cp.camera != nil {
		cp.camera.Close()
	}
	if cp.window != nil {
		cp.window.Close()
	}
	if cp.originalWindow != nil {
		cp.originalWindow.Close()
	}
}
