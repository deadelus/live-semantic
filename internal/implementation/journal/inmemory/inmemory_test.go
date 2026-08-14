package inmemory

import (
	"testing"
	"time"

	"live-semantic/internal/infrastructure/journal"
)

func TestJournal_RecordThenList(t *testing.T) {
	j := New()
	j.Record(journal.Entry{Timestamp: time.Now(), SessionID: "session-1", Type: "TrackEntered", TrackID: "track-1", Class: "person"})

	got := j.List()
	if len(got) != 1 || got[0].SessionID != "session-1" || got[0].Type != "TrackEntered" {
		t.Fatalf("List() = %+v, want 1 entry from session-1/TrackEntered", got)
	}
}

func TestJournal_List_EmptyIsEmpty(t *testing.T) {
	j := New()
	if got := j.List(); len(got) != 0 {
		t.Fatalf("List() on an empty journal = %+v, want empty", got)
	}
}

func TestJournal_List_NewestFirst(t *testing.T) {
	j := New()
	base := time.Now()
	j.Record(journal.Entry{Timestamp: base, Type: "TrackEntered", TrackID: "track-1"})
	j.Record(journal.Entry{Timestamp: base.Add(time.Second), Type: "TrackMatched", TrackID: "track-1"})
	j.Record(journal.Entry{Timestamp: base.Add(2 * time.Second), Type: "TrackLost", TrackID: "track-1"})

	got := j.List()
	if len(got) != 3 {
		t.Fatalf("List() = %d entries, want 3", len(got))
	}
	if got[0].Type != "TrackLost" || got[1].Type != "TrackMatched" || got[2].Type != "TrackEntered" {
		t.Fatalf("List() order = %v, want newest-first [TrackLost TrackMatched TrackEntered]", []string{got[0].Type, got[1].Type, got[2].Type})
	}
}

// TestJournal_EvictsOldestPastCapacity confirms the ring buffer actually
// bounds memory — maxEntries+50 records, only maxEntries survive, and
// they're the *most recent* ones (the whole point of a journal: recent
// history, not an arbitrary subset).
func TestJournal_EvictsOldestPastCapacity(t *testing.T) {
	j := New()
	base := time.Now()
	total := maxEntries + 50

	for i := 0; i < total; i++ {
		j.Record(journal.Entry{Timestamp: base.Add(time.Duration(i) * time.Millisecond), TrackID: "track-1", Type: "TrackMatched"})
	}

	got := j.List()
	if len(got) != maxEntries {
		t.Fatalf("List() has %d entries, want exactly maxEntries (%d)", len(got), maxEntries)
	}
	// Newest-first: the very first entry returned should be the very
	// last one recorded (index total-1), i.e. the oldest 50 were evicted.
	wantNewest := base.Add(time.Duration(total-1) * time.Millisecond)
	if !got[0].Timestamp.Equal(wantNewest) {
		t.Fatalf("newest entry timestamp = %v, want %v (the last one recorded)", got[0].Timestamp, wantNewest)
	}
	oldestKept := base.Add(time.Duration(total-maxEntries) * time.Millisecond)
	if !got[len(got)-1].Timestamp.Equal(oldestKept) {
		t.Fatalf("oldest surviving entry timestamp = %v, want %v (the 50 oldest evicted)", got[len(got)-1].Timestamp, oldestKept)
	}
}
