// Package gocvtracker implements tracking.ObjectTracker on top of OpenCV's
// contrib tracking module (KCF/CSRT), via gocv.io/x/gocv/contrib.
//
// MOSSE is deliberately not offered here: it is absent from this gocv
// binding (removed upstream in favor of KCF) — see
// docs/adr/object-tracking.md before adding it back.
package gocvtracker

import (
	"fmt"
	"time"

	"live-semantic/internal/domain/entities"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

// Algorithm selects which OpenCV tracking algorithm backs a Tracker. Which
// one performs best on real footage is an open question (TODO.md § B, drift
// test not run yet) — both are wired behind the same adapter so the choice
// is a one-line change, not a rewrite.
type Algorithm int

const (
	// KCF (Kernelized Correlation Filters): cheapest of the two, known to
	// drift on fast motion or scale change.
	KCF Algorithm = iota
	// CSRT (Discriminative Correlation Filter with Channel and Spatial
	// Reliability): costlier, generally more robust to occlusion/scale.
	CSRT
)

// String implements fmt.Stringer for Algorithm, used in error messages and
// logging.
func (a Algorithm) String() string {
	switch a {
	case KCF:
		return "KCF"
	case CSRT:
		return "CSRT"
	default:
		return "unknown"
	}
}

// Tracker implements tracking.ObjectTracker for a single algorithm choice.
// Not safe for concurrent use.
type Tracker struct {
	algorithm  Algorithm
	backend    gocv.Tracker
	label      string
	confidence float32
}

// sharedFrameMat caches the last image.Image -> gocv.Mat conversion, keyed
// by *entities.Frame pointer identity. gocv.ImageToMatRGB is a pure-Go,
// per-pixel loop — expensive on a real camera frame (~100ms measured on
// 1280x720, see TODO.md § F) — and every Tracker instance used to redo it
// independently on every Init/Update call. trackManager (application/uc/
// tracking.go) always passes the *same* *entities.Frame pointer to every
// active track within one advance()/reanchor() cycle (the video loop is
// single-threaded, confirmed by trackManager's own doc comment), so
// caching by pointer identity is safe and turns N conversions/frame into 1.
//
// Deliberately package-level, not per-Tracker: multiple Tracker instances
// (one per track) must share the same cache to benefit. The only
// consequence is a single Mat's worth of native memory held until the next
// distinct frame arrives — bounded, reclaimed by the OS at process exit,
// not worth a cross-package release hook for.
var sharedFrameMat struct {
	frame *entities.Frame
	mat   gocv.Mat
}

// matForFrame returns a gocv.Mat view of frame's image, converting once per
// distinct *entities.Frame and reusing it on subsequent calls with the same
// pointer. See sharedFrameMat's doc comment for the safety argument.
func matForFrame(frame *entities.Frame) (gocv.Mat, error) {
	if sharedFrameMat.frame == frame {
		return sharedFrameMat.mat, nil
	}

	start := time.Now()
	mat, err := gocv.ImageToMatRGB(frame.Image)
	// TEMP diagnostic (TODO.md § F perf investigation, 2026-08-09): confirm
	// the cache is actually hit in real usage and measure the conversion
	// cost at real camera resolution — remove once the perf question is
	// settled either way.
	bounds := frame.Image.Bounds()
	fmt.Printf("gocvtracker: cache MISS, converting frame (%dx%d) in %v\n", bounds.Dx(), bounds.Dy(), time.Since(start))
	if err != nil {
		return gocv.Mat{}, err
	}

	if sharedFrameMat.frame != nil {
		sharedFrameMat.mat.Close()
	}
	sharedFrameMat.frame = frame
	sharedFrameMat.mat = mat
	return mat, nil
}

// New creates a Tracker backed by the given algorithm. The underlying
// OpenCV tracker instance is only created on Init — OpenCV trackers are
// single-use (see gocv.Tracker.Init godoc: once lost, you must Close() and
// create a new instance, not reuse the old one), Init here recreates it
// every time to hide that constraint from callers.
func New(algorithm Algorithm) (*Tracker, error) {
	switch algorithm {
	case KCF, CSRT:
		return &Tracker{algorithm: algorithm}, nil
	default:
		return nil, fmt.Errorf("gocvtracker: unsupported algorithm %v", algorithm)
	}
}

func (t *Tracker) newBackend() gocv.Tracker {
	if t.algorithm == CSRT {
		return contrib.NewTrackerCSRT()
	}
	return contrib.NewTrackerKCF()
}

// Init implements tracking.ObjectTracker.Init for Tracker.
func (t *Tracker) Init(frame *entities.Frame, box entities.BoundingBox) error {
	if frame == nil || frame.Image == nil {
		return fmt.Errorf("gocvtracker: nil frame")
	}

	mat, err := matForFrame(frame)
	if err != nil {
		return fmt.Errorf("gocvtracker: image to mat: %w", err)
	}

	// Discard any previous backend before creating a new one (single-use
	// constraint, see New's doc comment).
	t.Cleanup()

	backend := t.newBackend()
	if ok := backend.Init(mat, box.ToRect()); !ok {
		backend.Close()
		return fmt.Errorf("gocvtracker: %s init failed on box %s", t.algorithm, box.ToString())
	}

	t.backend = backend
	t.label = box.Label
	t.confidence = box.Confidence
	return nil
}

// Update implements tracking.ObjectTracker.Update for Tracker. The returned
// bounding box keeps the Label/Confidence captured at Init time — the
// tracker itself only produces geometry, not a class or a score.
func (t *Tracker) Update(frame *entities.Frame) (entities.BoundingBox, bool) {
	if t.backend == nil || frame == nil || frame.Image == nil {
		return entities.BoundingBox{}, false
	}

	mat, err := matForFrame(frame)
	if err != nil {
		return entities.BoundingBox{}, false
	}

	// TEMP diagnostic (TODO.md § F perf investigation, 2026-08-09): isolate
	// backend.Update() (KCF/CSRT correlation filter) cost from conversion
	// cost — remove once the perf question is settled either way.
	backendStart := time.Now()
	rect, ok := t.backend.Update(mat)
	fmt.Printf("gocvtracker: %s backend.Update in %v\n", t.algorithm, time.Since(backendStart))
	if !ok {
		return entities.BoundingBox{}, false
	}

	return entities.BoundingBox{
		Label:      t.label,
		Confidence: t.confidence,
		X1:         float32(rect.Min.X),
		Y1:         float32(rect.Min.Y),
		X2:         float32(rect.Max.X),
		Y2:         float32(rect.Max.Y),
	}, true
}

// Cleanup implements tracking.ObjectTracker.Cleanup for Tracker. Safe to
// call multiple times or before Init.
func (t *Tracker) Cleanup() {
	if t.backend != nil {
		t.backend.Close()
		t.backend = nil
	}
}
