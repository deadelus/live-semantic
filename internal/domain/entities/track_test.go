package entities

import (
	"testing"
	"time"
)

func newTestBox(label string) BoundingBox {
	return BoundingBox{Label: label, Confidence: 0.9, X1: 0, Y1: 0, X2: 10, Y2: 10}
}

func TestNewTrackStartsTentative(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)

	if tr.State != StateTentative {
		t.Errorf("State = %v, want StateTentative", tr.State)
	}
	if tr.Class != "person" {
		t.Errorf("Class = %q, want %q", tr.Class, "person")
	}
	if len(tr.Trajectory) != 1 {
		t.Errorf("len(Trajectory) = %d, want 1", len(tr.Trajectory))
	}
	if tr.FirstSeen != now || tr.LastSeen != now {
		t.Errorf("FirstSeen/LastSeen not seeded from initial timestamp")
	}
}

// confirm advances tr to StateConfirmed via the minimum number of
// MatchDetection calls (NewTrack already counts as hit #1).
func confirm(tr *Track, now time.Time) {
	for i := 0; i < minHitsToConfirm-1; i++ {
		tr.MatchDetection(newTestBox(tr.Class), now, 0)
	}
}

func TestMatchDetectionPromotesToConfirmedAfterMinHits(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)

	var lastEvent *TrackEvent
	for i := 0; i < minHitsToConfirm-2; i++ {
		lastEvent = tr.MatchDetection(newTestBox("person"), now, 0)
		if lastEvent != nil {
			t.Fatalf("hit %d: unexpected event %v, want nil (still below minHitsToConfirm)", i+1, lastEvent.Type)
		}
		if tr.State != StateTentative {
			t.Fatalf("hit %d: State = %v, want StateTentative", i+1, tr.State)
		}
	}

	lastEvent = tr.MatchDetection(newTestBox("person"), now, 0)
	if lastEvent == nil || lastEvent.Type != EventTrackEntered {
		t.Fatalf("final hit: event = %v, want EventTrackEntered", lastEvent)
	}
	if tr.State != StateConfirmed {
		t.Errorf("State = %v, want StateConfirmed", tr.State)
	}
}

func TestConfirmedTrackMatchEmitsTrackMatched(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)
	confirm(tr, now)
	if tr.State != StateConfirmed {
		t.Fatalf("precondition failed: State = %v, want StateConfirmed", tr.State)
	}

	evt := tr.MatchDetection(newTestBox("person"), now, 0)
	if evt == nil || evt.Type != EventTrackMatched {
		t.Fatalf("event = %v, want EventTrackMatched", evt)
	}
}

func TestCoastMovesConfirmedToCoasting(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)
	confirm(tr, now)
	if tr.State != StateConfirmed {
		t.Fatalf("precondition failed: State = %v, want StateConfirmed", tr.State)
	}

	tr.Coast(newTestBox("person"), now)
	if tr.State != StateCoasting {
		t.Errorf("State = %v, want StateCoasting", tr.State)
	}
}

func TestMatchDetectionRecoversFromCoasting(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)
	confirm(tr, now)
	tr.Coast(newTestBox("person"), now)

	evt := tr.MatchDetection(newTestBox("person"), now, 0)
	if evt == nil || evt.Type != EventTrackMatched {
		t.Fatalf("event = %v, want EventTrackMatched", evt)
	}
	if tr.State != StateConfirmed {
		t.Errorf("State = %v, want StateConfirmed", tr.State)
	}
}

func TestMissTransitionsToLostAfterMaxMisses(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)

	var lastEvent *TrackEvent
	for i := 0; i < maxMissesBeforeLost-1; i++ {
		lastEvent = tr.Miss(now)
		if lastEvent != nil {
			t.Fatalf("miss %d: unexpected event %v, want nil", i+1, lastEvent.Type)
		}
		if tr.State == StateLost {
			t.Fatalf("miss %d: State went Lost too early", i+1)
		}
	}

	lastEvent = tr.Miss(now)
	if lastEvent == nil || lastEvent.Type != EventTrackLost {
		t.Fatalf("final miss: event = %v, want EventTrackLost", lastEvent)
	}
	if tr.State != StateLost {
		t.Errorf("State = %v, want StateLost", tr.State)
	}
}

func TestLostTrackIsTerminal(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)
	for i := 0; i < maxMissesBeforeLost; i++ {
		tr.Miss(now)
	}
	if tr.State != StateLost {
		t.Fatalf("precondition failed: State = %v, want StateLost", tr.State)
	}

	if evt := tr.Miss(now); evt != nil {
		t.Errorf("Miss on already-Lost track returned %v, want nil", evt)
	}
	tr.Coast(newTestBox("person"), now)
	if tr.State != StateLost {
		t.Errorf("Coast resurrected a Lost track: State = %v", tr.State)
	}
}

func TestMatchDetectionResetsMissStreak(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)
	tr.Miss(now)
	tr.Miss(now)
	if tr.misses != 2 {
		t.Fatalf("precondition failed: misses = %d, want 2", tr.misses)
	}

	tr.MatchDetection(newTestBox("person"), now, 0)
	if tr.misses != 0 {
		t.Errorf("misses = %d, want 0 after MatchDetection", tr.misses)
	}
}

func TestMatchDetectionThreadsScoreOntoEvent(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)
	confirm(tr, now)
	if tr.State != StateConfirmed {
		t.Fatalf("precondition failed: State = %v, want StateConfirmed", tr.State)
	}

	evt := tr.MatchDetection(newTestBox("person"), now, 0.31)
	if evt == nil {
		t.Fatal("event = nil, want EventTrackMatched")
	}
	if evt.Score != 0.31 {
		t.Errorf("Score = %v, want 0.31", evt.Score)
	}
}

func TestLastBox(t *testing.T) {
	now := time.Now()
	tr := NewTrack("t1", newTestBox("person"), now)
	second := BoundingBox{Label: "person", X1: 5, Y1: 5, X2: 15, Y2: 15}
	tr.MatchDetection(second, now, 0)

	got := tr.LastBox()
	if got != second {
		t.Errorf("LastBox() = %+v, want %+v", got, second)
	}
}

func TestTrackStateString(t *testing.T) {
	cases := map[TrackState]string{
		StateTentative: "Tentative",
		StateConfirmed: "Confirmed",
		StateCoasting:  "Coasting",
		StateLost:      "Lost",
		TrackState(99): "Unknown",
	}
	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("TrackState(%d).String() = %q, want %q", state, got, want)
		}
	}
}
