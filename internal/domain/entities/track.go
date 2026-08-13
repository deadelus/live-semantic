package entities

import "time"

// TrackState represents where a Track is in its lifecycle.
type TrackState int

const (
	// StateTentative: just created, not yet confirmed by enough consecutive
	// detection matches — avoids spawning a track for one spurious
	// detection.
	StateTentative TrackState = iota
	// StateConfirmed: matched by fresh detections consistently enough,
	// considered a real tracked object.
	StateConfirmed
	// StateCoasting: not matched by a fresh detection this cycle, position
	// held by the tracking-by-detection tracker alone between two
	// re-detections. Still alive, but degrading.
	StateCoasting
	// StateLost: too many consecutive misses (neither a fresh detection nor
	// the tracker could locate the object) — dead, callers should drop it.
	StateLost
)

// String implements fmt.Stringer for TrackState, useful for logging.
func (s TrackState) String() string {
	switch s {
	case StateTentative:
		return "Tentative"
	case StateConfirmed:
		return "Confirmed"
	case StateCoasting:
		return "Coasting"
	case StateLost:
		return "Lost"
	default:
		return "Unknown"
	}
}

// TrackEventType distinguishes the lifecycle transitions a Track can emit.
// Callers (application layer) translate these into AlertSender
// notifications instead of alerting on every single frame.
type TrackEventType int

const (
	// EventTrackEntered: a track was just confirmed for the first time
	// (Tentative → Confirmed).
	EventTrackEntered TrackEventType = iota
	// EventTrackMatched: an already-confirmed track was re-anchored by a
	// fresh detection (including recovering from Coasting).
	EventTrackMatched
	// EventTrackLost: a track transitioned to StateLost.
	EventTrackLost
)

// String implements fmt.Stringer for TrackEventType, useful for logging.
func (e TrackEventType) String() string {
	switch e {
	case EventTrackEntered:
		return "TrackEntered"
	case EventTrackMatched:
		return "TrackMatched"
	case EventTrackLost:
		return "TrackLost"
	default:
		return "Unknown"
	}
}

// TrackEvent is emitted by Track's state-transition methods when a
// lifecycle-significant change happens (as opposed to routine
// coasting/matching that doesn't change state). nil means "nothing worth
// alerting on happened".
type TrackEvent struct {
	Type  TrackEventType
	Track *Track

	// Score is the semantic match score (CLIP cosine similarity, once the
	// YOLO->crop->CLIP cascade lands) that led to this event, when
	// applicable — set by MatchDetection. 0 for events that don't stem
	// from a semantic match (EventTrackLost, or a match made with no
	// filter active, i.e. no CLIP score was ever computed for it). As of
	// 2026-08-10, alerts (application/uc.emit) report this instead of the
	// detector's own box.Confidence, which isn't what actually decided
	// whether this event happened.
	Score float32
}

// Tuning constants for the state machine.
const (
	minHitsToConfirm = 3

	// maxMissesBeforeLost: lowered 5 -> 2 on 2026-08-09, found in real
	// usage. Misses are almost always driven by reanchor() failing to
	// re-associate a track (Miss() from advance()'s own tracker.Update()
	// failure is rare in practice — CSRT in particular almost never
	// reports failure on its own, see docs/adr/object-tracking.md § 7:
	// 0 tracker_failures across both test videos, it just keeps reporting
	// a guess, often clamped to the frame edge, for whatever's now under
	// the last known region). With reanchor only running every
	// reanchorInterval frames, 5 meant a track that genuinely left frame
	// stayed drawn for ~5 reanchor cycles (~225 frames, several seconds)
	// before disappearing. 2 trades some flicker risk (a track dies after
	// 2 consecutive frames where YOLO just didn't detect it, e.g. brief
	// occlusion/motion blur) for a much faster response to the common case
	// of the object actually being gone.
	maxMissesBeforeLost = 2
)

// Track is the domain aggregate for one physical object followed across
// frames. It aggregates a class label, a position history and semantic
// embeddings (Embeddings stays empty until the CLIP cascade exists —
// nothing populates it yet).
type Track struct {
	ID         string
	Class      string
	State      TrackState
	Trajectory []BoundingBox
	Embeddings []Embedding
	FirstSeen  time.Time
	LastSeen   time.Time

	hits   int // consecutive successful matches (detection or tracker)
	misses int // consecutive frames with neither a detection nor tracker match
}

// NewTrack creates a new Track in StateTentative from an initial detection.
// id is caller-provided: Track stays dependency-free (no uuid generator),
// callers are responsible for uniqueness.
func NewTrack(id string, box BoundingBox, at time.Time) *Track {
	return &Track{
		ID:         id,
		Class:      box.Label,
		State:      StateTentative,
		Trajectory: []BoundingBox{box},
		FirstSeen:  at,
		LastSeen:   at,
		hits:       1,
	}
}

// MatchDetection records a fresh detection that was IoU-associated to this
// track (tracking-by-detection re-anchoring). Resets the miss streak; once
// enough consecutive hits accumulate, promotes StateTentative →
// StateConfirmed. score is the semantic match score (CLIP cosine
// similarity) that led to this
// association, if any (0 when no filter was active) — threaded onto the
// returned TrackEvent, not stored on the box itself (BoundingBox stays a
// generic domain type, not CLIP-specific). Returns the TrackEvent to alert
// on, or nil if the match didn't change anything worth alerting on (e.g.
// still Tentative, not enough hits yet).
func (t *Track) MatchDetection(box BoundingBox, at time.Time, score float32) *TrackEvent {
	wasTentative := t.State == StateTentative
	wasCoasting := t.State == StateCoasting

	t.Trajectory = append(t.Trajectory, box)
	t.LastSeen = at
	t.hits++
	t.misses = 0

	switch {
	case wasTentative && t.hits >= minHitsToConfirm:
		t.State = StateConfirmed
		return &TrackEvent{Type: EventTrackEntered, Track: t, Score: score}
	case wasTentative:
		return nil
	case wasCoasting:
		t.State = StateConfirmed
		return &TrackEvent{Type: EventTrackMatched, Track: t, Score: score}
	default:
		return &TrackEvent{Type: EventTrackMatched, Track: t, Score: score}
	}
}

// Coast records a tracker-only update (tracking-by-detection): no fresh
// detection matched this track this cycle, but the tracker still reports
// a position. Moves StateConfirmed → StateCoasting. No-op on a StateLost
// track.
func (t *Track) Coast(box BoundingBox, at time.Time) {
	if t.State == StateLost {
		return
	}
	t.Trajectory = append(t.Trajectory, box)
	t.LastSeen = at

	if t.State == StateConfirmed {
		t.State = StateCoasting
	}
}

// Miss records a frame where neither a fresh detection nor the tracker
// (tracking-by-detection) could locate the object. Past
// maxMissesBeforeLost consecutive misses, transitions to StateLost and
// returns the corresponding TrackEvent — nil otherwise, or if the track
// is already StateLost.
func (t *Track) Miss(at time.Time) *TrackEvent {
	if t.State == StateLost {
		return nil
	}
	t.misses++
	t.hits = 0

	if t.misses >= maxMissesBeforeLost {
		t.State = StateLost
		return &TrackEvent{Type: EventTrackLost, Track: t}
	}
	return nil
}

// AddEmbedding appends a semantic embedding to this track's aggregate.
// Populated once the YOLO->crop->CLIP cascade is integrated — no caller
// does this yet.
func (t *Track) AddEmbedding(e Embedding) {
	t.Embeddings = append(t.Embeddings, e)
}

// LastBox returns the most recently recorded bounding box for this track,
// or the zero value if the track has no history (shouldn't happen — NewTrack
// always seeds Trajectory with one box).
func (t *Track) LastBox() BoundingBox {
	if len(t.Trajectory) == 0 {
		return BoundingBox{}
	}
	return t.Trajectory[len(t.Trajectory)-1]
}
