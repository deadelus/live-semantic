// Package journal is the port for the aggregated multi-flux event log
// (docs/gui/mockups/ screen 1b's "Journal des événements" drawer,
// docs/gui/spec.md § 3.5 — "n'existe pas, zéro ligne de code" until
// 2026-08-14). Deliberately separate from notifier.AlertSender: that
// port is filtered/debounced by design (only fires for a set filter,
// skips EventTrackLost, notifyDebounce-throttled per track — see
// application/uc/tracking.go's emit(), tuned for "alert a human", not
// "keep a complete record") — a journal wants every lifecycle
// transition, unconditionally, which is a different contract entirely,
// not a stricter/looser version of the same one.
package journal

import "time"

// Entry is one recorded Track lifecycle transition — SessionID ties it
// back to the session.Manager session it came from (screen 1b's own
// per-source filter pills), Score is empty (0) for EventTrackLost and
// for a pure-COCO-label match (TrackEvent.Score's own doc comment: score
// only exists for a semantic match).
type Entry struct {
	Timestamp time.Time
	SessionID string
	Type      string // "TrackEntered" / "TrackMatched" / "TrackLost" — TrackEventType.String()
	TrackID   string
	Class     string
	Score     float32
}

// Journal records and lists Track lifecycle events across every session
// sharing this instance (session.Manager wires one shared Journal into
// every uc.UseCase it creates, same sharing rationale already documented
// for storage.GalleryStorage/CollectionStorage — a journal aggregated
// across sources is the whole point, not a per-session log). Thread-
// safety is each implementation's own concern.
type Journal interface {
	// Record appends entry — implementations may bound retention (age
	// and/or count) rather than growing forever, same rationale as
	// implementation/streamer/output.RingBufferOutput.
	Record(entry Entry)
	// List returns every currently-retained entry, newest first.
	List() []Entry
}
