package input

import (
	"fmt"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/streamer"

	"gocv.io/x/gocv"
)

var _ streamer.InputStream = (*FileInput)(nil)

// FileInput reads frames from any URI OpenCV's FFmpeg backend can open:
// a local video file path, or a network stream URL (rtsp://, http://,
// ...) — gocv.VideoCaptureFile handles both the same way, picking the
// backend from the URI scheme. TODO.md § H1 lists "fichier vidéo local"
// and "RTSP/caméra IP" as two separate adapters, but the open call and
// read loop are identical either way — one type, not two.
//
// Local files were already proven working in this project (cmd/tracking-
// drift, unrelated to this adapter but the same gocv.VideoCaptureFile
// call).
//
// RTSP verified end-to-end 2026-08-11 against a real (self-hosted, not
// public) RTSP session: no public demo RTSP stream is reliable enough to
// depend on (they rotate/go offline — confirmed by search, e.g. Wowza's
// demo URL changes over time), so the repeatable way to test this is
// self-hosting one locally — `brew install mediamtx`, run it, then loop
// an existing repo asset into it over RTSP:
//
//	mediamtx &
//	ffmpeg -re -stream_loop -1 -i assets/videos/car.mp4 -c copy -f rtsp rtsp://127.0.0.1:8554/car &
//	# then point FileInput at rtsp://127.0.0.1:8554/car
//
// Confirmed working this way: 96 frames read over a 5s window, correct
// 1280x720 dimensions, clean Cleanup(). FFmpeg logs a few "Missing
// reference picture"/"mmco: unref short failure" warnings on connect —
// expected (the client attaches mid-GOP on a live/looping H.264 stream,
// not specific to this adapter), decoding recovers within the first few
// frames. Not committed as an automated test: would need either a
// vendored RTSP server binary or a live network dependency, neither
// fits this project's existing test conventions — this doc comment is
// the reproduction recipe instead.
//
// Known gap (docs/gui/spec.md § 1.3, § 4): no reconnection handling. A
// local file simply ends (Read returns false, Start returns cleanly) —
// expected. A network stream dropping mid-session behaves identically
// today (silent stop) — wrong for a live camera, which should retry
// instead. Not implemented here; needs a real RTSP source to validate
// against before it's worth building.
type FileInput struct {
	uri     string
	capture *gocv.VideoCapture
	running bool
}

// NewFileInput creates a FileInput bound to the given URI (local path or
// network stream URL).
func NewFileInput(uri string) *FileInput {
	return &FileInput{uri: uri}
}

// Initialize implements the Input.Initialize for FileInput.
func (fi *FileInput) Initialize() error {
	var err error
	fi.capture, err = gocv.VideoCaptureFile(fi.uri)
	if err != nil {
		return fmt.Errorf("file input: opening %q: %w", fi.uri, err)
	}
	return nil
}

// Start implements the Input.Start for FileInput. Unlike CameraInput,
// frames are never mirrored — a recorded file or an IP camera's own feed
// must be shown as-is.
func (fi *FileInput) Start(frameActionCallback func(*entities.Frame) (*entities.Frame, error)) error {
	defer fi.Cleanup()

	if fi.capture == nil {
		return fmt.Errorf("file input not initialized")
	}
	fi.running = true
	captureLoop(fi.capture, false, func() bool { return fi.running }, frameActionCallback)
	return nil
}

// Stop implements the Input.Stop for FileInput.
func (fi *FileInput) Stop() {
	fi.running = false
}

// Cleanup implements the Input.Cleanup for FileInput.
func (fi *FileInput) Cleanup() {
	if fi.capture != nil {
		fi.capture.Close()
	}
	fmt.Println("FileInput resources cleaned up successfully.")
}
