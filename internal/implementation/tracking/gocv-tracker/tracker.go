// Package gocvtracker implements tracking.ObjectTracker on top of OpenCV's
// contrib tracking module (KCF/CSRT), via gocv.io/x/gocv/contrib.
//
// MOSSE is deliberately not offered here: it is absent from this gocv
// binding (removed upstream in favor of KCF) — see
// docs/adr/object-tracking.md before adding it back.
package gocvtracker

import (
	"fmt"
	"image"
	"os"
	"sync"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/tracking"

	"gocv.io/x/gocv"
	"gocv.io/x/gocv/contrib"
)

// init tunes OpenCV for real-time tracking of one webcam frame at a time,
// found across a real perf investigation on 2026-08-09 (this comment
// states the current understanding):
//
//   - OPENCV_OPENCL_DEVICE=disabled: KCF's updateProjectionMatrix dispatches
//     a matrix transpose to the GPU via OpenCV's T-API (UMat/OpenCL). On an
//     Intel Kaby Lake iGPU, `sample` (macOS CLI profiler) showed the GPU
//     sync alone (clFinish -> AppleIntelKBLGraphicsGLDriver -> mach IPC)
//     costing hundreds of ms — far more than doing the transpose on CPU.
//     Confirmed both via env var and by re-profiling after. Must be set
//     before any OpenCV/OpenCL call — os.Setenv here, in this package's
//     init(), runs before any gocv function does (OpenCL context is lazy,
//     created on first actual use).
//   - gocv.SetNumThreads(1): defensive, not confirmed to matter once the
//     OpenCL issue above is fixed — kept because it's cheap and a genuine
//     multi-track slowdown was observed before the real cause was found,
//     may still help avoid oversubscription with 2+ concurrent trackers.
func init() {
	os.Setenv("OPENCV_OPENCL_DEVICE", "disabled")
	gocv.SetNumThreads(1)
}

// maxTrackingDimension bounds the longest side (px) of the image actually
// fed to the tracking algorithm. KCF/CSRT's correlation filter is FFT-based
// — cost scales with the tracked region (and the frame it's read from),
// confirmed via `sample`: on a real webcam frame with someone standing
// close to the camera (large bounding box), denseGaussKernel/getSubWindow
// alone cost 150-200ms/frame even with OpenCL disabled — 20-40x the 2-9ms
// seen on cmd/tracking-drift-bench's test footage (smaller, farther subjects).
// Downscaling before tracking (standard practice in real-time CV — track
// on a cheap proxy, scale the result back up) trades a little geometric
// precision for a roughly quadratic reduction in FFT cost.
//
// Lowered 640 -> 320 on 2026-08-09 (real-world request, more speed traded
// for more precision loss) — not re-measured on real webcam footage at
// this value yet, only reasoned from the same quadratic-cost argument that
// held for 640. Revert or tune (perf follow-up) if tracking gets flaky on
// small/far subjects.
const maxTrackingDimension = 320

// scaleFor returns the downscale factor (<=1.0) to bring w or h under
// maxTrackingDimension, or 1.0 if already small enough (no upscaling).
func scaleFor(w, h int) float64 {
	longest := w
	if h > longest {
		longest = h
	}
	if longest <= maxTrackingDimension {
		return 1.0
	}
	return float64(maxTrackingDimension) / float64(longest)
}

// scaleRect scales r's corners by s, used both to bring a box into the
// downscaled tracking coordinate space (Init) and back out of it (Update).
func scaleRect(r image.Rectangle, s float64) image.Rectangle {
	return image.Rect(
		int(float64(r.Min.X)*s), int(float64(r.Min.Y)*s),
		int(float64(r.Max.X)*s), int(float64(r.Max.Y)*s),
	)
}

// Algorithm selects which OpenCV tracking algorithm backs a Tracker. Which
// one performs best on real footage is an open question (tracking-by-
// detection drift test not run yet) — both are wired behind the same
// adapter so the choice is a one-line change, not a rewrite.
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
	cache      *frameCache
}

// frameCache caches the last image.Image -> gocv.Mat conversion (already
// downscaled to maxTrackingDimension), keyed by *entities.Frame pointer
// identity. gocv.ImageToMatRGB is a pure-Go, per-pixel loop, and every
// Tracker instance used to redo it independently on every Init/Update call.
// trackManager (application/uc/tracking.go) always passes the *same*
// *entities.Frame pointer to every active track within one
// advance()/reanchor() cycle, so caching by pointer identity is safe and
// turns N conversions/frame into 1 — and doing the downscale here too means
// it also only happens once per frame, not once per track.
//
// One instance per Factory (see Factory's own doc comment), not a package-
// level global anymore — a real SIGSEGV found 2026-08-13 (multi-flux, two
// sessions running concurrently — one on the built-in webcam, one on an
// external/iPhone Continuity Camera) is why: with a single shared cache,
// one session's cycle could call matForFrame with a *different* frame
// pointer and Close() the Mat another session's Tracker.Update() was still
// mid-call on in native OpenCV/cgo code (Update() releases this cache's
// lock before the native call, see its own doc comment) — a use-after-free
// in C++ that segfaulted the whole process (goroutine trace: gocv/contrib
// TrackerSubclass_Update -> addr=0x0). trackManager's own lock
// (application/uc/tracking.go) only ever serialized calls *within one
// session* — it was never a cross-session guarantee, contrary to what the
// previous version of this doc comment assumed. Scoping one frameCache per
// Factory (and therefore per session, since Factory is now built fresh per
// session — see cmd/livesemantic/main.go) fixes this at the root instead
// of trying to serialize across sessions, which would defeat the point of
// running them concurrently.
//
// mu guards the cache fields themselves against the *intra-session*
// concurrent-call case (defense in depth — trackManager's own lock already
// serializes those in practice).
type frameCache struct {
	mu    sync.Mutex
	frame *entities.Frame
	mat   gocv.Mat
	scale float64
}

// matForFrame returns a downscaled gocv.Mat view of frame's image (see
// maxTrackingDimension) and the scale factor applied, converting once per
// distinct *entities.Frame and reusing it on subsequent calls with the same
// pointer. See frameCache's doc comment for the safety argument.
func (c *frameCache) matForFrame(frame *entities.Frame) (gocv.Mat, float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.frame == frame {
		return c.mat, c.scale, nil
	}

	full, err := gocv.ImageToMatRGB(frame.Image)
	if err != nil {
		return gocv.Mat{}, 0, err
	}

	scale := scaleFor(full.Cols(), full.Rows())
	mat := full
	if scale < 1.0 {
		resized := gocv.NewMat()
		if err := gocv.Resize(full, &resized, image.Point{}, scale, scale, gocv.InterpolationLinear); err != nil {
			full.Close()
			resized.Close()
			return gocv.Mat{}, 0, fmt.Errorf("gocvtracker: resize: %w", err)
		}
		full.Close()
		mat = resized
	}

	if c.frame != nil {
		c.mat.Close()
	}
	c.frame = frame
	c.mat = mat
	c.scale = scale
	return mat, scale, nil
}

// New creates a Tracker backed by the given algorithm, with its own
// private frameCache — a safe default for direct callers (tests, any
// non-Factory usage) that aren't otherwise sharing a cache with anything.
// Prefer going through a Factory (see its own doc comment) when multiple
// Trackers need to share one cache (e.g. every track within one session).
// The underlying OpenCV tracker instance is only created on Init — OpenCV
// trackers are single-use (see gocv.Tracker.Init godoc: once lost, you
// must Close() and create a new instance, not reuse the old one), Init
// here recreates it every time to hide that constraint from callers.
func New(algorithm Algorithm) (*Tracker, error) {
	return newWithCache(algorithm, &frameCache{})
}

// newWithCache is New with an explicit, possibly-shared cache — used by
// Factory.New so every Tracker it creates shares one frameCache instance
// (same session), while still never sharing across Factory instances
// (different sessions, see frameCache's doc comment).
func newWithCache(algorithm Algorithm, cache *frameCache) (*Tracker, error) {
	switch algorithm {
	case KCF, CSRT:
		return &Tracker{algorithm: algorithm, cache: cache}, nil
	default:
		return nil, fmt.Errorf("gocvtracker: unsupported algorithm %v", algorithm)
	}
}

// newBackend constructs the underlying gocv.Tracker for t's algorithm
// choice — a fresh instance every call, per New's single-use constraint.
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

	mat, scale, err := t.cache.matForFrame(frame)
	if err != nil {
		return fmt.Errorf("gocvtracker: image to mat: %w", err)
	}

	// Discard any previous backend before creating a new one (single-use
	// constraint, see New's doc comment).
	t.Cleanup()

	backend := t.newBackend()
	if ok := backend.Init(mat, scaleRect(box.ToRect(), scale)); !ok {
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

	mat, scale, err := t.cache.matForFrame(frame)
	if err != nil {
		return entities.BoundingBox{}, false
	}

	rect, ok := t.backend.Update(mat)
	if !ok {
		return entities.BoundingBox{}, false
	}
	// Scale back up to the original frame's coordinate space (mat is
	// downscaled to maxTrackingDimension, see matForFrame).
	rect = scaleRect(rect, 1/scale)

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

// Factory implements tracking.TrackerFactory for a fixed Algorithm choice
// — the composition root (cmd/livesemantic/main.go) builds one with the
// algorithm it wants and injects it, so application/uc never imports
// this package or knows an Algorithm value exists (dependency inversion
// — see tracking.TrackerFactory's doc comment).
//
// Owns one frameCache, shared by every Tracker this Factory instance
// creates — correct within one session (every track needs the same
// per-frame Mat conversion, frameCache's whole point) but, since
// 2026-08-13, deliberately NOT shared across Factory instances anymore:
// the composition root must build a fresh Factory per session (mono-
// session UseCase and each session.Manager entry alike) rather than
// reusing one Factory everywhere, or this cache would be right back to
// being a de facto global — see frameCache's doc comment for the SIGSEGV
// that cost.
type Factory struct {
	algorithm Algorithm
	cache     *frameCache
}

// NewFactory creates a Factory that always builds trackers using
// algorithm, with a fresh frameCache — call this once per session, not
// once for the whole process (see Factory's own doc comment).
func NewFactory(algorithm Algorithm) *Factory {
	return &Factory{algorithm: algorithm, cache: &frameCache{}}
}

var _ tracking.TrackerFactory = (*Factory)(nil)

// New implements tracking.TrackerFactory.
func (f *Factory) New() (tracking.ObjectTracker, error) {
	return newWithCache(f.algorithm, f.cache)
}
