package uc

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/tracking"
)

// normalizeFilter trims and lowercases a user-supplied class filter before
// comparing it against a COCO label (always lowercase, entities.class.go).
// Found in real usage (2026-08-09): an untrimmed CLI survey answer (a
// single leading space, "person" typed as " person") silently filtered out
// every detection — exact string comparison, no error, looked exactly like
// "detection is broken" from the user's side. Applied at every comparison
// site rather than trusting callers to normalize req.Filter once.
func normalizeFilter(filter string) string {
	return strings.ToLower(strings.TrimSpace(filter))
}

// iouAssociationThreshold: minimum IoU for a fresh detection to be
// considered the same object as an existing track (TODO.md § B calls for
// 0.3-0.5). Lowered 0.4 -> 0.3 on 2026-08-09: real-world duplicate tracks
// observed (same physical person spawning a 2nd/3rd/4th track every
// reanchor cycle instead of re-anchoring the existing one) — matches
// cmd/tracking-drift's own measurement of CSRT's min IoU (0.328) on
// person.mp4, just under the old 0.4 threshold. A failed match doesn't
// just miss a re-anchor, it spawns a visible duplicate box and the stale
// track lingers (maxMissesBeforeLost in entities/track.go) — worth staying
// at the permissive end of the documented range.
const iouAssociationThreshold = 0.3

// Note on reanchor cadence: there used to be an explicit
// defaultReanchorInterval (frame-count-based gate, plus a
// LIVESEMANTIC_REANCHOR_INTERVAL env var for A/B testing during the perf
// investigation, TODO.md § F). Both are gone as of the async pipeline
// (TODO.md § C, 2026-08-09): the detection loop now pulls from a buffer-1
// overwrite-on-full channel and processes whatever's latest as fast as it
// can — reanchor's own cost (~150-270ms measured) self-paces it to roughly
// the target 2-5 FPS without needing an explicit interval. See
// uc_recognition.go's RecognitionUseCase doc comment for the full picture.

// trackedObject pairs a domain Track with the tracker instance following it
// between re-detections. One per active track — trackers are single-object,
// can't be shared (see gocv-tracker's doc).
type trackedObject struct {
	track   *entities.Track
	tracker tracking.ObjectTracker

	// lastNotifiedAt debounces AlertSender.Notify for this track — see
	// notifyDebounce's doc comment.
	lastNotifiedAt time.Time
}

// notifyDebounce: minimum time between two Notify calls for the same
// track. Found necessary after decoupling the detection loop (TODO.md § C,
// 2026-08-10): reanchor now runs continuously (~200ms cadence, self-paced
// by its own cost) instead of being gated by a frame count tied to camera
// FPS (~1-2s before) — every reanchor cycle that still matches a confirmed
// track emits EventTrackMatched, which used to alert roughly once every
// 1-2s and now would alert ~5x/s. That directly contradicts the original
// intent (TODO.md § D: "one alert per meaningful lifecycle transition
// instead of one per frame") — reanchor became frequent enough to
// effectively become "per frame" again from the alerting point of view.
const notifyDebounce = 5 * time.Second

// trackManager owns the set of active tracks, shared between the video loop
// (advance/boxes, per frame) and the detection loop (reanchor, whenever a
// frame is handed off — TODO.md § C, the async 3-loop pipeline). mu serializes
// every public method: coarse-grained on purpose for a first version — the
// underlying tracker.ObjectTracker instances are themselves not safe for
// concurrent use (native OpenCV handles), so advance() and reanchor() must
// never touch the same trackedObject at once regardless of map-level
// locking granularity. The actual win from splitting the loops isn't
// parallelism (there's none, gocv is intentionally single-threaded — see
// gocv-tracker's SetNumThreads(1)), it's that AnalyzeFrame (YOLO, the
// dominant cost of reanchor) runs fully unlocked, so advance()/render()
// only ever wait for the comparatively cheap post-processing, never the
// full detection call.
type trackManager struct {
	uc          *UseCase
	mu          sync.Mutex
	active      map[string]*trackedObject
	nextTrackID uint64
}

func newTrackManager(uc *UseCase) *trackManager {
	return &trackManager{uc: uc, active: make(map[string]*trackedObject)}
}

// count returns the number of active tracks — safe accessor for logging
// from either loop (TODO.md § C), avoids reaching into m.active directly
// from outside the package/lock.
func (m *trackManager) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.active)
}

// boxes returns the current bounding box of every active track, for
// drawing — regardless of state (Tentative tracks are drawn too, they're
// still a real detection, just not confirmed as stable yet).
func (m *trackManager) boxes() []entities.BoundingBox {
	m.mu.Lock()
	defer m.mu.Unlock()

	boxes := make([]entities.BoundingBox, 0, len(m.active))
	for _, obj := range m.active {
		boxes = append(boxes, obj.track.LastBox())
	}
	return boxes
}

// cleanup releases every active tracker. Call once when the video loop
// exits.
func (m *trackManager) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, obj := range m.active {
		obj.tracker.Cleanup()
	}
	m.active = make(map[string]*trackedObject)
}

// advance updates every active track from the tracker alone (no fresh
// detection this frame) — the cheap path run on most frames between
// re-detections.
func (m *trackManager) advance(frame *entities.Frame, req dto.RecognitionRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	bounds := frame.Image.Bounds()
	for id, obj := range m.active {
		box, ok := obj.tracker.Update(frame)
		// KCF/CSRT almost never report failure (ok=false) when the
		// physical object leaves the frame — they just keep reporting a
		// guess for whatever's now under the last known region. Found in
		// real usage: a track's box stayed on screen long after the
		// person walked out of frame, because the only other way to drop
		// a track is failing re-anchor association enough times in a row
		// (entities.maxMissesBeforeLost), which only gets evaluated once
		// per detection-loop cycle — several seconds of a stale box before
		// this check existed. A box entirely outside the frame is an
		// unambiguous, immediate signal, no need to wait for that.
		if ok && !boxWithinFrame(box, bounds) {
			ok = false
		}
		if !ok {
			m.miss(id, obj, now, req)
			continue
		}
		obj.track.Coast(box, now)
	}
}

// boxWithinFrame reports whether box has any overlap at all with bounds —
// false once the tracker's reported position has drifted (or the real
// object walked) fully outside the visible frame.
func boxWithinFrame(box entities.BoundingBox, bounds image.Rectangle) bool {
	return box.X2 > float32(bounds.Min.X) && box.X1 < float32(bounds.Max.X) &&
		box.Y2 > float32(bounds.Min.Y) && box.Y1 < float32(bounds.Max.Y)
}

// reanchor runs a full YOLO detection and IoU-associates the results
// against active tracks, spawning new tracks for unmatched detections and
// missing tracks that weren't matched this cycle. The expensive path — runs
// on the detection loop's own goroutine, not gated by a frame count
// (TODO.md § C, see uc_recognition.go's RecognitionUseCase doc comment).
//
// AnalyzeFrame deliberately runs before the lock is taken: it's the
// dominant cost (~150-270ms measured) and touches no trackManager state at
// all (result is a local value) — locking around it would mean advance()
// on the video loop blocks for the full YOLO duration on every reanchor
// cycle, defeating the point of running detection on a separate goroutine.
func (m *trackManager) reanchor(frame *entities.Frame, req dto.RecognitionRequest) error {
	result, err := m.uc.ai.AnalyzeFrame(frame)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	matchedTrackIDs := make(map[string]bool, len(m.active))

	for _, box := range result.BoundingBoxes {
		// Only track what was actually requested — req.Filter == "" means
		// "everything" (e.g. transports with no filter concept yet), but a
		// real filter must actually restrict what gets tracked/drawn, not
		// just what gets alerted on (that part alone was already handled
		// by emit(), the rest of the pipeline ignored req.Filter entirely
		// until now — a lingering pre-tracking bug, the filter check block
		// used to be dead code commented out in the original per-frame loop).
		if filter := normalizeFilter(req.Filter); filter != "" && box.Label != filter {
			continue
		}

		if id, ok := m.bestMatch(box, matchedTrackIDs); ok {
			obj := m.active[id]
			evt := obj.track.MatchDetection(box, now)
			// Re-anchor the tracker itself on the fresh detection to wipe
			// out any drift accumulated since the last re-detection.
			if err := obj.tracker.Init(frame, box); err != nil {
				m.uc.logger.Info("Tracker re-init failed", map[string]interface{}{"track_id": id, "error": err.Error()})
			}
			matchedTrackIDs[id] = true
			m.emit(obj, evt, req)
			continue
		}

		m.spawn(frame, box, now)
	}

	for id, obj := range m.active {
		if matchedTrackIDs[id] {
			continue
		}
		m.miss(id, obj, now, req)
	}

	return nil
}

// bestMatch returns the active track with the highest IoU against box
// (restricted to the same class, above iouAssociationThreshold, and not
// already matched this cycle). Greedy per-detection association, not a
// global optimum (Hungarian algorithm) — sufficient for a first version,
// revisit if the drift test (TODO.md § B) shows association errors.
//
// bestMatch/spawn/miss/emit below are internal helpers: only ever called
// from advance()/reanchor() while m.mu is already held. They don't lock
// themselves — sync.Mutex isn't reentrant, a second Lock() from the same
// goroutine would deadlock.
func (m *trackManager) bestMatch(box entities.BoundingBox, taken map[string]bool) (string, bool) {
	bestID := ""
	bestIoU := float32(iouAssociationThreshold)

	for id, obj := range m.active {
		if taken[id] || obj.track.Class != box.Label {
			continue
		}
		last := obj.track.LastBox()
		if iou := box.IoU(&last); iou >= bestIoU {
			bestIoU = iou
			bestID = id
		}
	}

	return bestID, bestID != ""
}

// spawn creates a new track + tracker for a detection that didn't match any
// existing track. A brand-new track starts StateTentative (NewTrack already
// counts the initial detection as hit #1) — no TrackEvent to emit yet.
func (m *trackManager) spawn(frame *entities.Frame, box entities.BoundingBox, now time.Time) {
	trk, err := m.uc.trackerFactory()
	if err != nil {
		m.uc.logger.Info("Tracker creation failed", map[string]interface{}{"error": err.Error()})
		return
	}
	if err := trk.Init(frame, box); err != nil {
		m.uc.logger.Info("Tracker init failed", map[string]interface{}{"error": err.Error()})
		trk.Cleanup()
		return
	}

	m.nextTrackID++
	id := fmt.Sprintf("track-%d", m.nextTrackID)
	m.active[id] = &trackedObject{track: entities.NewTrack(id, box, now), tracker: trk}
}

// miss records a track that wasn't matched this cycle (neither by the
// tracker in advance(), nor by a detection in reanchor()) and drops it once
// it transitions to StateLost.
func (m *trackManager) miss(id string, obj *trackedObject, now time.Time, req dto.RecognitionRequest) {
	evt := obj.track.Miss(now)
	m.emit(obj, evt, req)
	if obj.track.State == entities.StateLost {
		obj.tracker.Cleanup()
		delete(m.active, id)
	}
}

// emit logs every track event and forwards it to AlertSender when the
// track's class matches the requested filter above the similarity
// threshold — one alert per meaningful lifecycle transition (TODO.md § D)
// instead of the old per-frame alert, debounced per track (notifyDebounce)
// since the async detection loop (TODO.md § C) made EventTrackMatched fire
// far more often than the original "per lifecycle transition" intent.
func (m *trackManager) emit(obj *trackedObject, evt *entities.TrackEvent, req dto.RecognitionRequest) {
	if evt == nil {
		return
	}

	m.uc.logger.Info("Track event", map[string]interface{}{
		"type":  evt.Type.String(),
		"id":    evt.Track.ID,
		"class": evt.Track.Class,
		"state": evt.Track.State.String(),
	})

	if evt.Type == entities.EventTrackLost {
		return
	}

	box := evt.Track.LastBox()
	filter := normalizeFilter(req.Filter)
	if filter == "" || box.Label != filter || box.Confidence < req.SimilarityThreshold {
		return
	}

	now := time.Now()
	if !obj.lastNotifiedAt.IsZero() && now.Sub(obj.lastNotifiedAt) < notifyDebounce {
		return
	}
	obj.lastNotifiedAt = now

	if err := m.uc.notifier.Notify(entities.Message{
		MatchedFilter: filter,
		Confidence:    box.Confidence,
	}); err != nil {
		m.uc.logger.Info("Notify failed", map[string]interface{}{"error": err.Error()})
	}
}
