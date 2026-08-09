// Package tracking defines the port for tracking-by-detection between two
// re-detections (see TODO.md § B). No implementation exists yet — target is
// a gocv-native tracker (KCF, CSRT or MOSSE, choice pending a drift test on
// real video).
package tracking

import (
	"live-semantic/internal/domain/entities"
)

// ObjectTracker is the port for single-object tracking: initialized on a
// detected bounding box, then advanced frame-by-frame in between periodic
// re-detections.
type ObjectTracker interface {
	// Init (re)starts tracking the given bounding box in frame.
	Init(frame *entities.Frame, box entities.BoundingBox) error
	// Update advances the tracker to the next frame, returning the updated
	// bounding box and whether the track is still considered valid (false
	// signals drift/loss — caller should re-anchor via detection).
	Update(frame *entities.Frame) (entities.BoundingBox, bool)
	// Cleanup releases any resources held by the tracker (e.g. native OpenCV
	// tracker handles, which must be closed manually). Safe to call even if
	// Init was never called.
	Cleanup()
}
