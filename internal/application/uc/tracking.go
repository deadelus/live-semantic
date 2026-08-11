package uc

import (
	"fmt"
	"image"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/tracking"
)

// normalizeFilter trims and lowercases a user-supplied filter term before
// comparing/hashing it. Found in real usage (2026-08-09): an untrimmed CLI
// survey answer (a single leading space, "person" typed as " person")
// silently filtered out every detection — exact string comparison, no
// error, looked exactly like "detection is broken" from the user's side.
// Applied at every comparison site rather than trusting callers to
// normalize req.Filter once.
func normalizeFilter(filter string) string {
	return strings.ToLower(strings.TrimSpace(filter))
}

// iouAssociationThreshold: minimum IoU for a fresh detection to be
// considered the same object as an existing track (TODO.md § B calls for
// 0.3-0.5). Lowered 0.4 -> 0.3 on 2026-08-09: real-world duplicate tracks
// observed (same physical person spawning a 2nd/3rd/4th track every
// reanchor cycle instead of re-anchoring the existing one) — matches
// cmd/tracking-drift-bench's own measurement of CSRT's min IoU (0.328) on
// person.mp4, just under the old 0.4 threshold. A failed match doesn't
// just miss a re-anchor, it spawns a visible duplicate box and the stale
// track lingers (maxMissesBeforeLost in entities/track.go) — worth staying
// at the permissive end of the documented range.
const iouAssociationThreshold = 0.3

// defaultSimilarityThreshold gates a semantic (non-COCO) filter term's CLIP
// cosine similarity (TODO.md § A, docs/adr/clip-backend.md § 13). Hardcoded
// on purpose (2026-08-11): the user wants a fixed default for now, not a
// CLI/API knob — that's meant to become a GUI control (TODO.md § H) once
// there's a live-feedback slider to make the threshold's fragility
// (§ 7/§ 10, margins of 0.01-0.03) something a user can actually see and
// react to, rather than a number typed blind into a prompt. Still the same
// underlying fragility as before — this default doesn't fix that, it just
// isn't exposed as a knob right now.
const defaultSimilarityThreshold float32 = 0.20

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

	// filterKey is the filterTerm.Key that caused this track to spawn —
	// the cap-counting bucket (countByFilterKey), not necessarily the same
	// as track.Class (a semantic term's Key is free text, e.g. "person
	// with a red hat", while track.Class is still whatever COCO label YOLO
	// itself gave the box, e.g. "person"). Empty string when no filter was
	// active at all (no cap to enforce).
	filterKey string

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

// termMatch is how trackManager remembers a parsed filterTerm internally:
// Embedding nil means an exact COCO-label term (matched directly against
// box.Label, no CLIP call); non-nil means a semantic term (matched via
// CLIP cosine similarity against every still-unclaimed candidate box,
// ranked, capped — see reanchor).
type termMatch struct {
	Cap       int
	Embedding entities.Embedding
	// Overlap mirrors filterTerm.Overlap ("+overlap" in the filter spec):
	// when true, this term's semantic pass (reanchor) may still evaluate a
	// candidate box another term already claimed this cycle. Consulted by
	// reanchor's pass 2 — see its doc comment.
	Overlap bool
	// LabelHint (filter_spec.go's semanticLabelHint) restricts this term's
	// candidates to boxes YOLO already labeled with this class — empty
	// means no restriction, every box is a candidate. Only ever set for a
	// semantic term (Embedding != nil); a nil Embedding never has a hint.
	LabelHint string
}

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

	// terms maps each filterTerm.Key to its cap/embedding (filter_spec.go),
	// parsed/encoded once here (not per frame) rather than in NewUseCase:
	// the filter is only known per RecognitionUseCase call. nil/empty
	// means "no filter requested" — reanchor then accepts every box, never
	// calls CLIP.
	//
	// History (TODO.md § A, docs/adr/clip-backend.md § 12-13): CLIP-only
	// (2026-08-10) → exact-label-only (2026-08-11 morning, CLIP's absolute
	// threshold was rejecting real matches at any value tried) → this
	// hybrid (2026-08-11 afternoon, user's explicit request): a term that
	// is a COCO label matches exactly (no threshold fragility for the 80
	// classes YOLO already knows by name); a term that isn't matches
	// semantically via CLIP against whatever candidates aren't already
	// claimed by an exact term this cycle.
	terms map[string]termMatch
}

// newTrackManager parses the filter spec once here (not per frame), before
// the video/detection loops start: cheap for exact terms (pure string
// parsing), one ONNX EncodeText call per semantic term for the rest — still
// the right place for it either way, since the filter is only known per
// RecognitionUseCase call, not at NewUseCase time.
func newTrackManager(uc *UseCase, req dto.RecognitionRequest) (*trackManager, error) {
	m := &trackManager{uc: uc, active: make(map[string]*trackedObject)}

	parsed, err := parseFilterSpec(req.Filter)
	if err != nil {
		return nil, fmt.Errorf("invalid filter %q: %w", req.Filter, err)
	}
	if len(parsed) == 0 {
		return m, nil
	}

	m.terms = make(map[string]termMatch, len(parsed))
	for _, t := range parsed {
		if isCOCOLabel(t.Key) {
			m.terms[t.Key] = termMatch{Cap: t.Cap, Overlap: t.Overlap}
			continue
		}
		emb, err := uc.semanticEncoder.EncodeText(t.Key)
		if err != nil {
			return nil, fmt.Errorf("encode semantic filter term %q: %w", t.Key, err)
		}
		labelHint, _ := semanticLabelHint(t.Key)
		m.terms[t.Key] = termMatch{Cap: t.Cap, Embedding: emb, Overlap: t.Overlap, LabelHint: labelHint}
	}

	return m, nil
}

// cosineSimilarity returns the cosine similarity of a and b, in [-1, 1] for
// well-formed embeddings (0 for mismatched/empty inputs). Doesn't assume
// its inputs are already unit vectors (clip.Encoder happens to L2-normalize
// its output, but the SemanticEncoder port doesn't contractually guarantee
// that — computing the true cosine similarity here keeps this package
// decoupled from that implementation detail).
func cosineSimilarity(a, b entities.Embedding) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
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

// scoredCandidate pairs a still-unclaimed YOLO box (by index into the
// original detection result) with its CLIP score against one semantic
// term — reanchor's semantic pass ranks these to enforce a term's cap by
// "best matches first" rather than detection order.
type scoredCandidate struct {
	index int
	box   entities.BoundingBox
	score float32
}

// reanchor runs a full YOLO detection, gates/associates the results against
// the active filter (exact COCO-label terms first, then semantic/CLIP terms
// on whatever's left, TODO.md § A), and IoU-associates survivors against
// active tracks — spawning new tracks for unmatched detections and missing
// tracks that weren't matched this cycle. The expensive path — runs on the
// detection loop's own goroutine, not gated by a frame count (TODO.md § C,
// see uc_recognition.go's RecognitionUseCase doc comment).
//
// AnalyzeFrame deliberately runs before the lock is taken: it's the
// dominant cost (~150-270ms measured) and touches no trackManager state at
// all (result is a local value) — locking around it would mean advance()
// on the video loop blocks for the full YOLO duration on every reanchor
// cycle, defeating the point of running detection on a separate goroutine.
func (m *trackManager) reanchor(frame *entities.Frame, req dto.RecognitionRequest) error {
	result, err := m.uc.objectDetector.AnalyzeFrame(frame)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	matchedTrackIDs := make(map[string]bool, len(m.active))

	if len(m.terms) == 0 {
		// No filter requested: track everything YOLO finds, no cap, no
		// CLIP call — pre-filter behavior. filterKey doesn't matter here
		// (no cap will ever be checked against it), box.Label is as good
		// a bucket as any for bookkeeping consistency.
		for _, box := range result.BoundingBoxes {
			m.matchOrSpawn(frame, box, box.Label, 0, now, matchedTrackIDs, req)
		}
		m.missUnmatched(now, matchedTrackIDs, req)
		return nil
	}

	claimed := make([]bool, len(result.BoundingBoxes))

	// Pass 1: exact COCO-label terms — a candidate whose own YOLO label is
	// directly requested is claimed immediately, no CLIP involved.
	//
	// Deliberate default, confirmed by the user 2026-08-11: a box claimed
	// here is excluded from every semantic term's candidate pool this
	// cycle UNLESS that semantic term opts in with "+overlap" (see pass 2
	// below) — "person*1" alongside "person with a red hat*1" never draws
	// two boxes on the same physical person by default, but does if the
	// second term is "person with a red hat*1+overlap" (TODO.md § A,
	// docs/adr/clip-backend.md § 13/16/17).
	for i, box := range result.BoundingBoxes {
		term, ok := m.terms[box.Label]
		if !ok || term.Embedding != nil {
			continue
		}
		claimed[i] = true
		m.matchOrSpawn(frame, box, box.Label, term.Cap, now, matchedTrackIDs, req)
	}

	// Pass 2: semantic terms — CLIP-scored, ranked, top Cap kept (the top-N
	// selection is what makes the cap meaningful for a scored match, "the
	// single best match" for "person with a red hat"*1, not just "first
	// one YOLO happened to list").
	//
	// Candidate pool per term, in order:
	//   - Skip a box already claimed this cycle (by pass 1 or another pass
	//     2 term) UNLESS this term has Overlap=true (its own "+overlap").
	//     A box this term does match is still marked claimed afterward —
	//     a later term without Overlap still won't see it, only a term
	//     that explicitly opts in can.
	//   - Skip a box whose own YOLO label doesn't match this term's
	//     LabelHint, when it has one (semanticLabelHint — a COCO class
	//     name mentioned in the term's free text, e.g. "person" in "person
	//     with a red hat"). Found necessary 2026-08-11, tested live: without
	//     this, "person with a yellow hat" was scored against every
	//     detected box including a couch and a potted plant, which
	//     sometimes outscored the actual person (docs/adr/clip-backend.md
	//     § 17). No hint (0 or 2+ COCO classes mentioned) means every box
	//     is still a candidate, same as before.
	for key, term := range m.terms {
		if term.Embedding == nil {
			continue
		}

		var candidates []scoredCandidate
		for i, box := range result.BoundingBoxes {
			if claimed[i] && !term.Overlap {
				continue
			}
			if term.LabelHint != "" && box.Label != term.LabelHint {
				continue
			}
			crop, ok := frame.Crop(box)
			if !ok {
				continue
			}
			embedding, err := m.uc.semanticEncoder.EncodeImage(crop)
			if err != nil {
				m.uc.logger.Info("Semantic encode failed", map[string]interface{}{"error": err.Error()})
				continue
			}
			score := cosineSimilarity(embedding, term.Embedding)
			// Logged regardless of whether it clears the threshold — the
			// only visibility into *why* a semantic term matched (or
			// didn't) something surprising, e.g. TODO.md § A's "potted
			// plant" case. Cheap relative to the EncodeImage call above.
			m.uc.logger.Info("Semantic candidate scored", map[string]interface{}{
				"term":            key,
				"yolo_label":      box.Label,
				"score":           score,
				"above_threshold": score >= defaultSimilarityThreshold,
			})
			if score < defaultSimilarityThreshold {
				continue
			}
			candidates = append(candidates, scoredCandidate{index: i, box: box, score: score})
		}

		sort.Slice(candidates, func(a, b int) bool { return candidates[a].score > candidates[b].score })

		for _, c := range candidates {
			claimed[c.index] = true
			m.matchOrSpawn(frame, c.box, key, term.Cap, now, matchedTrackIDs, req)
		}
	}

	m.missUnmatched(now, matchedTrackIDs, req)
	return nil
}

// matchOrSpawn re-anchors box onto its best-matching existing track (always
// allowed, regardless of cap — a cap only ever blocks a *new* spawn, TODO.md
// § A/I), or spawns a new track for it if the filterKey's cap (0 = no cap)
// isn't already reached. Only ever called from reanchor() while m.mu is
// already held.
func (m *trackManager) matchOrSpawn(frame *entities.Frame, box entities.BoundingBox, filterKey string, capVal int, now time.Time, matchedTrackIDs map[string]bool, req dto.RecognitionRequest) {
	if id, ok := m.bestMatch(box, matchedTrackIDs); ok {
		obj := m.active[id]
		// score is always 0 now — TrackEvent.Score used to carry the CLIP
		// score that gated the match (2026-08-10 design); matching is
		// exact-or-already-ranked-and-filtered by the time this runs, so
		// there's no additional score to thread through here. Kept as a
		// TrackEvent field for whatever eventually reintroduces a
		// per-match score (galerie de références, TODO.md § D).
		evt := obj.track.MatchDetection(box, now, 0)
		// Re-anchor the tracker itself on the fresh detection to wipe out
		// any drift accumulated since the last re-detection.
		if err := obj.tracker.Init(frame, box); err != nil {
			m.uc.logger.Info("Tracker re-init failed", map[string]interface{}{"track_id": id, "error": err.Error()})
		}
		matchedTrackIDs[id] = true
		m.emit(obj, evt, req)
		return
	}

	// Scene cap (TODO.md § A/I): once this term already has as many active
	// tracks as it allows, an unmatched extra candidate doesn't spawn a
	// new one — the "noise" this drops is exactly what a future event/
	// action system (not built) would hook into, per the user's request.
	if capVal > 0 && m.countByFilterKey(filterKey) >= capVal {
		return
	}

	if id, ok := m.spawn(frame, box, filterKey, now); ok {
		// Found 2026-08-11 while wiring +overlap: a freshly spawned track
		// MUST be marked matched this same cycle. Without this, two things
		// went wrong — (1) a later term this same reanchor() call (e.g. an
		// "+overlap" semantic term processed after an exact term already
		// spawned this box) could bestMatch its way onto the brand-new
		// track instead of spawning its own, since bestMatch only checks
		// matchedTrackIDs; (2) missUnmatched (called once, after every
		// term this cycle) would call Miss() on this track in the very
		// cycle it was born, resetting its hit streak back to 0 — silently
		// doubling how long every single spawn takes to reach
		// StateConfirmed (minHitsToConfirm), for every filter, not just
		// +overlap. Neither was caught by the test suite before the
		// +overlap tests exercised two terms matching the same box in one
		// cycle.
		matchedTrackIDs[id] = true
	}
}

// missUnmatched records a miss for every active track that wasn't matched
// this reanchor cycle. Only ever called from reanchor() while m.mu is
// already held.
func (m *trackManager) missUnmatched(now time.Time, matchedTrackIDs map[string]bool, req dto.RecognitionRequest) {
	for id, obj := range m.active {
		if matchedTrackIDs[id] {
			continue
		}
		m.miss(id, obj, now, req)
	}
}

// bestMatch returns the active track with the highest IoU against box
// (restricted to the same class, above iouAssociationThreshold, and not
// already matched this cycle). Greedy per-detection association, not a
// global optimum (Hungarian algorithm) — sufficient for a first version,
// revisit if the drift test (TODO.md § B) shows association errors.
//
// bestMatch/spawn/miss/emit below are internal helpers: only ever called
// from advance()/reanchor() (via matchOrSpawn) while m.mu is already held.
// They don't lock themselves — sync.Mutex isn't reentrant, a second Lock()
// from the same goroutine would deadlock.
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

// countByFilterKey returns how many active tracks currently belong to this
// filter term — used to enforce a term's cap (TODO.md § A/I). Keyed by
// filterTerm.Key (trackedObject.filterKey), not by COCO class: for an
// exact term the two happen to be the same string, but a semantic term's
// key is free text, not a class name, so it needs its own bucket.
func (m *trackManager) countByFilterKey(key string) int {
	n := 0
	for _, obj := range m.active {
		if obj.filterKey == key {
			n++
		}
	}
	return n
}

// spawn creates a new track + tracker for a detection that didn't match any
// existing track. A brand-new track starts StateTentative (NewTrack already
// counts the initial detection as hit #1) — no TrackEvent to emit yet.
// Returns the new track's ID and true on success — false (with a zero id)
// if tracker creation/init failed, in which case no track exists to add to
// matchedTrackIDs.
func (m *trackManager) spawn(frame *entities.Frame, box entities.BoundingBox, filterKey string, now time.Time) (string, bool) {
	trk, err := m.uc.trackerFactory()
	if err != nil {
		m.uc.logger.Info("Tracker creation failed", map[string]interface{}{"error": err.Error()})
		return "", false
	}
	if err := trk.Init(frame, box); err != nil {
		m.uc.logger.Info("Tracker init failed", map[string]interface{}{"error": err.Error()})
		trk.Cleanup()
		return "", false
	}

	m.nextTrackID++
	id := fmt.Sprintf("track-%d", m.nextTrackID)
	m.active[id] = &trackedObject{track: entities.NewTrack(id, box, now), tracker: trk, filterKey: filterKey}
	return id, true
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

// emit logs every track event and forwards it to AlertSender — one alert
// per meaningful lifecycle transition (TODO.md § D) instead of the old
// per-frame alert, debounced per track (notifyDebounce) since the async
// detection loop (TODO.md § C) made EventTrackMatched fire far more often
// than the original "per lifecycle transition" intent.
//
// No re-check of the filter here on purpose: by construction, every track
// that exists already passed the gate in reanchor() (exact label or CLIP
// threshold) — or no filter was requested at all, in which case there's
// nothing to alert on either, that's what the req.Filter == "" check below
// is for, not a redundant re-check.
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

	if normalizeFilter(req.Filter) == "" {
		return
	}

	now := time.Now()
	if !obj.lastNotifiedAt.IsZero() && now.Sub(obj.lastNotifiedAt) < notifyDebounce {
		return
	}
	obj.lastNotifiedAt = now

	// MatchedFilter reports the specific filter term that matched
	// (obj.filterKey — a COCO label for an exact term, free text for a
	// semantic one), not the raw multi-term filter spec (req.Filter can
	// list several terms, e.g. "person*2,car"). Confidence/evt.Score is
	// always 0 (see matchOrSpawn's doc comment) — kept as a field on
	// entities.Message for whatever eventually reintroduces a per-match
	// score (galerie de références, TODO.md § D).
	if err := m.uc.notifier.Notify(entities.Message{
		MatchedFilter: obj.filterKey,
		Confidence:    evt.Score,
	}); err != nil {
		m.uc.logger.Info("Notify failed", map[string]interface{}{"error": err.Error()})
	}
}
