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

// defaultDifferentialMargin gates a compound semantic term (one with a
// LabelHint, e.g. "person with a hat" -> LabelHint "person") on top of
// defaultSimilarityThreshold: the term's score must exceed its own base
// noun's score (term.BaseEmbedding, CLIP-encoded from LabelHint alone) by
// at least this margin. Added 2026-08-12 after a real bug (docs/adr/
// clip-backend.md § 23-24): a bare "person" already scores ~0.235-0.238 in
// real conditions (§ 10) — above defaultSimilarityThreshold on its own — so
// an absolute threshold alone can't tell "with a hat" apart from no hat at
// all, since the score stays dominated by the base noun regardless (tested
// live: two boxes drawn with or without an actual hat). Requiring a margin
// over the base noun's own score targets that specifically. Applied as an
// AND with defaultSimilarityThreshold, not a replacement — a low absolute
// score is still rejected outright even if the delta over an even-lower
// base score looks large, keeping every existing single-noun semantic term
// (no LabelHint, BaseEmbedding nil) and every prior real-webcam-validated
// scenario unaffected. Starting value NOT validated against real webcam
// data yet — same calibration risk as defaultSimilarityThreshold's own
// history (§ 7/§ 10), to be adjusted from real testing, not assumed
// correct as-is.
const defaultDifferentialMargin float32 = 0.02

// relationContainmentThreshold: minimum fraction of an attachment box's own
// area that must fall inside a container box for reanchor's relational
// pass ("%+%", docs/adr/clip-backend.md § 24) to count it as contained.
// Starting value, NOT validated against real webcam data yet — same
// calibration risk/discipline as defaultSimilarityThreshold and
// defaultDifferentialMargin (§ 7/§ 10/§ 23): to be adjusted from real
// testing (e.g. "person%+%backpack"), not assumed correct as-is.
const relationContainmentThreshold = 0.5

// containmentRatio returns the fraction of attachment's own area that
// overlaps container, in [0, 1] — deliberately NOT IoU: a small attachment
// (e.g. a backpack) fully inside a much larger container (e.g. a person)
// would score a low IoU by construction (union dominated by the
// container's area) despite being entirely contained, which is exactly
// the case this relation needs to recognize. 0 for a zero-area attachment
// box rather than dividing by zero.
func containmentRatio(attachment, container entities.BoundingBox) float32 {
	area := float32(attachment.RectArea())
	if area == 0 {
		return 0
	}
	return attachment.Intersection(&container) / area
}

// boxGap returns the pixel distance between a's and b's nearest edges —
// 0 if they touch or overlap. Used by the "near" relation operator
// (docs/adr/clip-backend.md § 27) as a proximity metric, distinct from
// containmentRatio's "+": deliberately edge-to-edge, not center-to-
// center — two large boxes with far-apart centers can still be
// edge-adjacent (e.g. a person and a car parked right next to them),
// which "near" should recognize as close; a center-distance metric would
// penalize large boxes unfairly relative to small ones for the same
// physical closeness.
func boxGap(a, b entities.BoundingBox) float32 {
	ra, rb := a.ToRect(), b.ToRect()

	dx := float32(0)
	switch {
	case ra.Max.X < rb.Min.X:
		dx = float32(rb.Min.X - ra.Max.X)
	case rb.Max.X < ra.Min.X:
		dx = float32(ra.Min.X - rb.Max.X)
	}

	dy := float32(0)
	switch {
	case ra.Max.Y < rb.Min.Y:
		dy = float32(rb.Min.Y - ra.Max.Y)
	case rb.Max.Y < ra.Min.Y:
		dy = float32(ra.Min.Y - rb.Max.Y)
	}

	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

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

	// lastScore is the CLIP cosine similarity that most recently matched
	// this track to a semantic term's candidate — 0 for an exact-term or
	// no-filter track, which never has a CLIP score at all (label match
	// isn't scored). Found necessary 2026-08-11: the box label used to
	// show YOLO's own box.Confidence for every track regardless of term,
	// which for a semantic term isn't the number that actually decided
	// the match and can misleadingly look like a strong, confident result
	// (e.g. "85%") when the real CLIP margin was ~0.25-0.28 — exactly the
	// documented threshold fragility (docs/adr/clip-backend.md § 7/§ 14),
	// just hidden from view instead of shown honestly.
	lastScore float32

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
	// BaseEmbedding is the CLIP text embedding of LabelHint alone (e.g.
	// "person" for the term "person with a hat") — nil unless LabelHint is
	// set. Used by reanchor's pass 2 for differential scoring
	// (defaultDifferentialMargin) instead of/on top of an absolute
	// threshold — see that constant's doc comment for why.
	BaseEmbedding entities.Embedding
	// Relation, RelationParam, Container, Attachment mirror filterTerm's
	// same-named fields — set only for a relational term
	// ("container%relation[=param]%attachment", e.g. "person%+%backpack",
	// docs/adr/clip-backend.md § 24). Relation == "" for every other term;
	// when set, Embedding/BaseEmbedding/LabelHint above are unused (no
	// CLIP call at all for this term — v1 requires Container/Attachment to
	// both be exact COCO labels, checked once at parseFilterSpec time).
	// Consulted by reanchor's new relational pass — see its doc comment.
	Relation      string
	RelationParam float32
	Container     string
	Attachment    string
	// Shared mirrors filterTerm.Shared — see its doc comment. Consulted
	// by reanchor's relational pass to switch the default greedy 1:1
	// pairing to N:M.
	Shared bool
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
		if t.Relation != "" {
			// Relational term ("person%+%backpack") — Key is the canonical
			// relation string, not itself a COCO label or free text, so it
			// must be checked (and handled) before either branch below.
			// No CLIP call: v1 requires both sides to already be exact
			// COCO labels (parseFilterSpec validated this), matched
			// geometrically by reanchor's relational pass, not scored.
			m.terms[t.Key] = termMatch{
				Cap: t.Cap, Overlap: t.Overlap,
				Relation: t.Relation, RelationParam: t.RelationParam,
				Container: t.Container, Attachment: t.Attachment,
				Shared: t.Shared,
			}
			continue
		}
		if isCOCOLabel(t.Key) {
			m.terms[t.Key] = termMatch{Cap: t.Cap, Overlap: t.Overlap}
			continue
		}
		// Gallery lookup before falling back to CLIP text encoding — a
		// third term family alongside COCO labels and free text
		// (TODO.md § D/§ H1, docs/adr/clip-backend.md § 24): a name
		// registered via AddGalleryReference is matched image↔image
		// against candidates (reanchor's pass 2, unchanged — it doesn't
		// care whether Embedding came from EncodeText or a gallery
		// lookup) instead of text↔image. Checked after isCOCOLabel
		// (unconditional COCO priority, same reason ReferenceGallery.Add
		// rejects a COCO-colliding name) but before EncodeText, so a
		// registered reference never pays for/risks a text-encode error.
		// No LabelHint/BaseEmbedding for a gallery term — there's no free
		// text to scan for a mentioned COCO class, and no "base noun" to
		// diff against (defaultDifferentialMargin, § 23) — falls back to
		// the absolute threshold alone, same as any other term with no
		// LabelHint.
		if emb, ok := uc.gallery.Get(t.Key); ok {
			m.terms[t.Key] = termMatch{Cap: t.Cap, Embedding: emb, Overlap: t.Overlap}
			continue
		}
		emb, err := uc.semanticEncoder.EncodeText(t.Key)
		if err != nil {
			return nil, fmt.Errorf("encode semantic filter term %q: %w", t.Key, err)
		}
		labelHint, _ := semanticLabelHint(t.Key)
		match := termMatch{Cap: t.Cap, Embedding: emb, Overlap: t.Overlap, LabelHint: labelHint}
		if labelHint != "" {
			baseEmb, err := uc.semanticEncoder.EncodeText(labelHint)
			if err != nil {
				return nil, fmt.Errorf("encode base term %q for filter term %q: %w", labelHint, t.Key, err)
			}
			match.BaseEmbedding = baseEmb
		}
		m.terms[t.Key] = match
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

// trackedBox pairs a track's current position with the identity that
// should drive how it's drawn — see boxes' doc comment for why this can't
// just be entities.BoundingBox.Label.
type trackedBox struct {
	Box       entities.BoundingBox
	FilterKey string
	TrackID   string
	// Score is the CLIP cosine similarity that matched this track, for a
	// semantic term — 0 for an exact-term/no-filter track (see
	// trackedObject.lastScore). Callers should display this instead of
	// Box.Confidence whenever it's non-zero: Box.Confidence is YOLO's own
	// detection confidence, unrelated to (and typically far higher-looking
	// than) whatever CLIP score actually decided a semantic match.
	Score float32
}

// boxes returns the current bounding box of every active track, for
// drawing — regardless of state (Tentative tracks are drawn too, they're
// still a real detection, just not confirmed as stable yet).
//
// Returns FilterKey (trackedObject.filterKey) alongside each box, not just
// entities.BoundingBox — found necessary 2026-08-11, tested live: two
// tracks matched to the same physical box (an exact term + a "+overlap"
// semantic term both on the same person) both carry the *same* underlying
// YOLO box.Label ("person" for both — that's the detector's label, not the
// filter term that matched), so drawing keyed on box.Label alone drew two
// visually identical, exactly overlapping rectangles — indistinguishable
// from a single box, even though two tracks genuinely existed
// (docs/adr/clip-backend.md § 18). Callers should key color/label off
// FilterKey instead.
func (m *trackManager) boxes() []trackedBox {
	m.mu.Lock()
	defer m.mu.Unlock()

	boxes := make([]trackedBox, 0, len(m.active))
	for id, obj := range m.active {
		boxes = append(boxes, trackedBox{Box: obj.track.LastBox(), FilterKey: obj.filterKey, TrackID: id, Score: obj.lastScore})
	}
	return boxes
}

// cascadeOverlapIoU/cascadeStepPx tune cascadeOffsets — see its doc
// comment. 0.85 is deliberately close to 1.0 (near-identical, not just
// "close"): this must only catch genuine duplicates on the same physical
// object (e.g. an exact term + a "+overlap" semantic term both anchored to
// the same YOLO box), not two different nearby objects that happen to
// overlap a bit, which should stay drawn at their real position.
const (
	cascadeOverlapIoU = float32(0.85)
	cascadeStepPx     = float32(16)
)

// cascadeOffsets returns, for each entry in boxes (same index), a pixel
// offset to add to all four of its box coordinates (X1/Y1/X2/Y2) before
// drawing. Requested by the user 2026-08-11 (docs/adr/clip-backend.md
// § 18-19): two tracks matched to the same physical object (e.g.
// "person*1" + "person with a red hat*1+overlap") are now drawn with
// different colors/labels (boxes' doc comment) but still at the exact
// same coordinates — same rectangle, same label position, one still
// hides the other's text. Boxes found to mutually overlap above
// cascadeOverlapIoU are staggered diagonally by cascadeStepPx per rank,
// cheap-window-manager style, so every label stays readable.
//
// Deterministic by TrackID (sorted first), not by boxes' slice order —
// trackManager.boxes() iterates a Go map, whose order is randomized per
// call. Ranking by raw slice order would make the stagger direction/
// amount flicker between frames for the exact same two tracks; ranking by
// TrackID keeps a given track at the same stack position across frames.
func cascadeOffsets(boxes []trackedBox) []float32 {
	order := make([]int, len(boxes))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		return boxes[order[a]].TrackID < boxes[order[b]].TrackID
	})

	offsets := make([]float32, len(boxes))
	for rank, idx := range order {
		box := boxes[idx].Box
		stack := 0
		for _, priorIdx := range order[:rank] {
			prior := boxes[priorIdx].Box
			if prior.IoU(&box) >= cascadeOverlapIoU {
				stack++
			}
		}
		offsets[idx] = float32(stack) * cascadeStepPx
	}
	return offsets
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
			m.matchOrSpawn(frame, box, box.Label, 0, 0, now, matchedTrackIDs, req)
		}
		m.missUnmatched(now, matchedTrackIDs, req)
		return nil
	}

	claimed := make([]bool, len(result.BoundingBoxes))

	// Pass 0: relational terms ("container%relation%attachment", e.g.
	// "person%+%backpack") — geometric decomposition rather than a single
	// CLIP score on a composed phrase (TODO.md § A, docs/adr/
	// clip-backend.md § 24: CLIP's compound-phrase scoring is dominated by
	// the base noun, § 22-23 — decomposing into two independently-detected
	// boxes plus a geometric check sidesteps that entirely). Runs before
	// pass 1/2 — a relational match is the most specific condition a
	// filter can express (two boxes AND a spatial relation between them),
	// reasonable to let it claim boxes first over a plain single-condition
	// term elsewhere in the same filter.
	//
	// Two operators today: "+" (containment — an attachment box counts as
	// "in" a container box when relationContainmentThreshold of the
	// attachment's own area falls inside the container; not IoU, a small
	// attachment like a backpack inside a much larger container like a
	// person would score a low IoU by construction — union dominated by
	// the person's area — despite being fully contained) and "near"
	// (proximity — the pixel gap between the two boxes' nearest edges,
	// docs/adr/clip-backend.md § 27, must be under the term's own
	// RelationParam; edge-to-edge rather than center-to-center, see
	// boxGap's doc comment).
	//
	// Cardinality (docs/adr/clip-backend.md § 24/29): every valid pair is
	// its own match, greedy 1:1 by default per cycle (a box claimed by
	// one pair can't join a second this cycle) — "+shared" (term.Shared)
	// switches this to N:M, letting a box join multiple pairs. Ranked
	// best-first either way (highest containment ratio, or shortest gap
	// for "near" — see the sort below), top Cap pairs kept. Both boxes in
	// a kept pair are marked claimed regardless of Shared — pass 1/2
	// never separately reclaim them (no "+overlap" equivalent for a
	// relational term yet either).
	for key, term := range m.terms {
		if term.Relation == "" {
			continue
		}

		type relPair struct {
			containerIdx, attachmentIdx int
			metric                      float32 // meaning depends on term.Relation — see the dispatch below
		}

		// Counted independently of the pairing loop below (not inside its
		// nested iteration, which would multiply-count e.g. the same
		// attachment once per container candidate) — distinguishes "one
		// side was never even detected by YOLO this cycle" from "both
		// detected but the geometry doesn't satisfy containment" (logged
		// per pair below). Found necessary 2026-08-12: the first real
		// webcam test of this pass gave zero visibility into why
		// "person%+%backpack" wasn't matching.
		containerCandidates, attachmentCandidates := 0, 0
		for i, box := range result.BoundingBoxes {
			if claimed[i] {
				continue
			}
			if box.Label == term.Container {
				containerCandidates++
			}
			if box.Label == term.Attachment {
				attachmentCandidates++
			}
		}
		if containerCandidates == 0 || attachmentCandidates == 0 {
			m.uc.logger.Info("Relational term has no candidates this cycle", map[string]interface{}{
				"term":                  key,
				"container":             term.Container,
				"attachment":            term.Attachment,
				"container_candidates":  containerCandidates,
				"attachment_candidates": attachmentCandidates,
			})
		}

		var pairs []relPair
		for ci, cbox := range result.BoundingBoxes {
			if claimed[ci] || cbox.Label != term.Container {
				continue
			}
			for ai, abox := range result.BoundingBoxes {
				if ai == ci || claimed[ai] || abox.Label != term.Attachment {
					continue
				}

				// metric's meaning and accept condition depend on the
				// operator — containment ratio (higher = better, "in") vs.
				// proximity gap (lower = better, "close"). Both logged
				// regardless of whether they clear their own accept
				// condition, same rationale as pass 2's "Semantic
				// candidate scored": the only visibility into why a
				// relational term didn't match something that looked
				// close in the video.
				var metric float32
				var accepted bool
				switch term.Relation {
				case "near":
					metric = boxGap(abox, cbox)
					accepted = metric <= term.RelationParam
				default: // "+"
					metric = containmentRatio(abox, cbox)
					accepted = metric >= relationContainmentThreshold
				}
				m.uc.logger.Info("Relational candidate scored", map[string]interface{}{
					"term":       key,
					"relation":   term.Relation,
					"container":  term.Container,
					"attachment": term.Attachment,
					"metric":     metric,
					"accepted":   accepted,
				})
				if !accepted {
					continue
				}
				pairs = append(pairs, relPair{containerIdx: ci, attachmentIdx: ai, metric: metric})
			}
		}
		// "near" ranks shortest gap first (ascending); every other
		// operator ("+") ranks highest metric first (descending) — same
		// two-branch dispatch as the scoring switch above.
		if term.Relation == "near" {
			sort.Slice(pairs, func(a, b int) bool { return pairs[a].metric < pairs[b].metric })
		} else {
			sort.Slice(pairs, func(a, b int) bool { return pairs[a].metric > pairs[b].metric })
		}

		usedContainer := make(map[int]bool, len(pairs))
		usedAttachment := make(map[int]bool, len(pairs))
		kept := 0
		for _, p := range pairs {
			if kept >= term.Cap {
				break
			}
			// Default cardinality is greedy 1:1 — a box already spent on
			// a kept pair this term can't join a second one.
			// "+shared" (term.Shared) lifts that: the same container
			// and/or attachment box may satisfy multiple pairs, e.g. one
			// unattended bag near several people should surface a pair
			// per person, not just the single closest/best one
			// (docs/adr/clip-backend.md § 24). usedContainer/
			// usedAttachment are still populated either way below —
			// harmless bookkeeping when Shared, meaningful exclusion
			// when not.
			if !term.Shared && (usedContainer[p.containerIdx] || usedAttachment[p.attachmentIdx]) {
				continue
			}
			usedContainer[p.containerIdx] = true
			usedAttachment[p.attachmentIdx] = true
			claimed[p.containerIdx] = true
			claimed[p.attachmentIdx] = true
			m.uc.logger.Info("Relational candidate matched", map[string]interface{}{
				"term":       key,
				"relation":   term.Relation,
				"container":  term.Container,
				"attachment": term.Attachment,
				"metric":     p.metric,
			})
			m.matchOrSpawn(frame, result.BoundingBoxes[p.containerIdx], key, term.Cap, 0, now, matchedTrackIDs, req)
			kept++
		}
	}

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
		m.matchOrSpawn(frame, box, box.Label, term.Cap, 0, now, matchedTrackIDs, req)
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
			accepted := score >= defaultSimilarityThreshold
			logFields := map[string]interface{}{
				"term":       key,
				"yolo_label": box.Label,
				"score":      score,
			}
			// Differential gate (defaultDifferentialMargin, see its doc
			// comment) on top of the absolute threshold — only for a
			// compound term that has a base noun to compare against
			// (BaseEmbedding, set whenever LabelHint is). A term without a
			// LabelHint (e.g. a genuinely open-ended concept, no COCO word
			// mentioned) has no base to diff against — falls back to the
			// absolute threshold alone, unchanged.
			if term.BaseEmbedding != nil {
				baseScore := cosineSimilarity(embedding, term.BaseEmbedding)
				delta := score - baseScore
				logFields["base_score"] = baseScore
				logFields["delta"] = delta
				accepted = accepted && delta >= defaultDifferentialMargin
			}
			logFields["above_threshold"] = accepted
			// Logged regardless of whether it's accepted — the only
			// visibility into *why* a semantic term matched (or didn't)
			// something surprising, e.g. TODO.md § A's "potted plant" case.
			// Cheap relative to the EncodeImage call above.
			m.uc.logger.Info("Semantic candidate scored", logFields)
			if !accepted {
				continue
			}
			candidates = append(candidates, scoredCandidate{index: i, box: box, score: score})
		}

		sort.Slice(candidates, func(a, b int) bool { return candidates[a].score > candidates[b].score })

		for _, c := range candidates {
			claimed[c.index] = true
			m.matchOrSpawn(frame, c.box, key, term.Cap, c.score, now, matchedTrackIDs, req)
		}
	}

	m.missUnmatched(now, matchedTrackIDs, req)
	return nil
}

// matchOrSpawn re-anchors box onto its best-matching existing track (always
// allowed, regardless of cap — a cap only ever blocks a *new* spawn, TODO.md
// § A/I), or spawns a new track for it if the filterKey's cap (0 = no cap)
// isn't already reached. score is the CLIP cosine similarity that produced
// this candidate for a semantic term (0 for an exact-term/no-filter call,
// which never has one) — threaded onto the TrackEvent and stored on the
// track for display (trackedObject.lastScore), not just discarded, so the
// UI shows the number that actually decided a semantic match instead of
// YOLO's unrelated box.Confidence (docs/adr/clip-backend.md § 20 — found
// misleading in real use: a semantic match displayed the same ~85% as an
// exact match, when its real CLIP margin was ~0.25-0.28). Only ever called
// from reanchor() while m.mu is already held.
func (m *trackManager) matchOrSpawn(frame *entities.Frame, box entities.BoundingBox, filterKey string, capVal int, score float32, now time.Time, matchedTrackIDs map[string]bool, req dto.RecognitionRequest) {
	if id, ok := m.bestMatch(box, filterKey, matchedTrackIDs); ok {
		obj := m.active[id]
		evt := obj.track.MatchDetection(box, now, score)
		obj.lastScore = score
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

	if id, ok := m.spawn(frame, box, filterKey, score, now); ok {
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
// (restricted to the same class AND the same filterKey, above
// iouAssociationThreshold, and not already matched this cycle). Greedy
// per-detection association, not a global optimum (Hungarian algorithm) —
// sufficient for a first version, revisit if the drift test (TODO.md § B)
// shows association errors.
//
// filterKey restriction added 2026-08-11 (docs/adr/clip-backend.md § 21) —
// a real bug, not hypothetical: an exact term and a "+overlap" semantic
// term matched to the same physical object spawn two tracks with the
// *same* track.Class (both "person", from the same YOLO box.Label) but
// different filterKey. Without also checking filterKey here, whichever
// term's matchOrSpawn call happened to run first would bestMatch its way
// onto *either* track (map iteration order over m.active is randomized),
// not necessarily its own — score/state updates (MatchDetection,
// obj.lastScore) then landed on the wrong track: the CLIP score computed
// for the semantic term could get written onto the exact term's track
// (and vice versa, exact's score=0 overwriting the semantic track's real
// score) — exactly what produced the swapped percentages the user saw
// (§ 21: exact term showing ~21%, semantic term showing ~91%, backwards).
// filterKey is fixed at spawn and never changes, so this restriction is
// safe: a term's calls now only ever re-anchor a track *it* created.
//
// bestMatch/spawn/miss/emit below are internal helpers: only ever called
// from advance()/reanchor() (via matchOrSpawn) while m.mu is already held.
// They don't lock themselves — sync.Mutex isn't reentrant, a second Lock()
// from the same goroutine would deadlock.
func (m *trackManager) bestMatch(box entities.BoundingBox, filterKey string, taken map[string]bool) (string, bool) {
	bestID := ""
	bestIoU := float32(iouAssociationThreshold)

	for id, obj := range m.active {
		if taken[id] || obj.track.Class != box.Label || obj.filterKey != filterKey {
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
func (m *trackManager) spawn(frame *entities.Frame, box entities.BoundingBox, filterKey string, score float32, now time.Time) (string, bool) {
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
	m.active[id] = &trackedObject{track: entities.NewTrack(id, box, now), tracker: trk, filterKey: filterKey, lastScore: score}
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
