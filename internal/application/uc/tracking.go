package uc

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

const (
	// defaultReanchorInterval: how many frames between full YOLO
	// re-detections. Between re-detections, active tracks are advanced via
	// the cheaper per-object tracker (TODO.md § B). Not measured yet
	// (TODO.md § F) — chosen to land roughly every 1.5s on a typical 30fps
	// webcam feed.
	defaultReanchorInterval = 45

	// iouAssociationThreshold: minimum IoU for a fresh detection to be
	// considered the same object as an existing track (TODO.md § B calls
	// for 0.3-0.5, picked the middle).
	iouAssociationThreshold = 0.4
)

// reanchorInterval reads LIVESEMANTIC_REANCHOR_INTERVAL if set (TEMP, perf
// investigation TODO.md § F, 2026-08-09) — e.g. =1 forces YOLO on every
// frame (pre-tracking behavior), for an easy A/B against the default 45
// without a rebuild. Falls back to defaultReanchorInterval otherwise.
func reanchorInterval() int {
	if v := os.Getenv("LIVESEMANTIC_REANCHOR_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultReanchorInterval
}

// trackedObject pairs a domain Track with the tracker instance following it
// between re-detections. One per active track — trackers are single-object,
// can't be shared (see gocv-tracker's doc).
type trackedObject struct {
	track   *entities.Track
	tracker tracking.ObjectTracker
}

// trackManager owns the set of active tracks for one RecognitionUseCase run.
// Not safe for concurrent use — the video loop is single-threaded today
// (TODO.md § C, the async 3-loop pipeline, doesn't exist yet).
type trackManager struct {
	uc          *UseCase
	active      map[string]*trackedObject
	nextTrackID uint64
}

func newTrackManager(uc *UseCase) *trackManager {
	return &trackManager{uc: uc, active: make(map[string]*trackedObject)}
}

// boxes returns the current bounding box of every active track, for
// drawing — regardless of state (Tentative tracks are drawn too, they're
// still a real detection, just not confirmed as stable yet).
func (m *trackManager) boxes() []entities.BoundingBox {
	boxes := make([]entities.BoundingBox, 0, len(m.active))
	for _, obj := range m.active {
		boxes = append(boxes, obj.track.LastBox())
	}
	return boxes
}

// cleanup releases every active tracker. Call once when the video loop
// exits.
func (m *trackManager) cleanup() {
	for _, obj := range m.active {
		obj.tracker.Cleanup()
	}
	m.active = make(map[string]*trackedObject)
}

// advance updates every active track from the tracker alone (no fresh
// detection this frame) — the cheap path run on most frames between
// re-detections.
func (m *trackManager) advance(frame *entities.Frame, req dto.RecognitionRequest) {
	now := time.Now()
	for id, obj := range m.active {
		box, ok := obj.tracker.Update(frame)
		if !ok {
			m.miss(id, obj, now, req)
			continue
		}
		obj.track.Coast(box, now)
	}
}

// reanchor runs a full YOLO detection and IoU-associates the results
// against active tracks, spawning new tracks for unmatched detections and
// missing tracks that weren't matched this cycle. The expensive path, run
// every reanchorInterval frames.
func (m *trackManager) reanchor(frame *entities.Frame, req dto.RecognitionRequest) error {
	result, err := m.uc.ai.AnalyzeFrame(frame)
	if err != nil {
		return err
	}

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
			m.emit(evt, req)
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
	m.emit(evt, req)
	if obj.track.State == entities.StateLost {
		obj.tracker.Cleanup()
		delete(m.active, id)
	}
}

// emit logs every track event and forwards it to AlertSender when the
// track's class matches the requested filter above the similarity
// threshold — one alert per meaningful lifecycle transition (TODO.md § D)
// instead of the old per-frame alert.
func (m *trackManager) emit(evt *entities.TrackEvent, req dto.RecognitionRequest) {
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

	if err := m.uc.notifier.Notify(entities.Message{
		MatchedFilter: filter,
		Confidence:    box.Confidence,
	}); err != nil {
		m.uc.logger.Info("Notify failed", map[string]interface{}{"error": err.Error()})
	}
}
