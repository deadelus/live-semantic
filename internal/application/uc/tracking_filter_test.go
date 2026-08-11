package uc

import (
	"testing"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
)

// newTestTrackedObject builds a trackedObject with just enough state for
// acceptsLabel/countByLabel to exercise — no real tracker.ObjectTracker
// needed, neither function touches it.
func newTestTrackedObject(id, label string) *trackedObject {
	box := entities.BoundingBox{Label: label}
	return &trackedObject{track: entities.NewTrack(id, box, time.Now())}
}

func TestAcceptsLabel(t *testing.T) {
	tests := []struct {
		name        string
		filterTerms map[string]int
		label       string
		want        bool
	}{
		{"no filter accepts everything", nil, "person", true},
		{"empty filter map accepts everything", map[string]int{}, "car", true},
		{"listed label accepted", map[string]int{"person": 1}, "person", true},
		{"unlisted label rejected", map[string]int{"person": 1}, "car", false},
		{"one of several listed labels accepted", map[string]int{"person": 2, "car": 1}, "car", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &trackManager{filterTerms: tt.filterTerms}
			if got := m.acceptsLabel(tt.label); got != tt.want {
				t.Fatalf("acceptsLabel(%q) = %v, want %v", tt.label, got, tt.want)
			}
		})
	}
}

func TestCountByLabel(t *testing.T) {
	m := &trackManager{
		active: map[string]*trackedObject{
			"track-1": newTestTrackedObject("track-1", "person"),
			"track-2": newTestTrackedObject("track-2", "person"),
			"track-3": newTestTrackedObject("track-3", "car"),
		},
	}

	if got := m.countByLabel("person"); got != 2 {
		t.Fatalf("countByLabel(person) = %d, want 2", got)
	}
	if got := m.countByLabel("car"); got != 1 {
		t.Fatalf("countByLabel(car) = %d, want 1", got)
	}
	if got := m.countByLabel("dog"); got != 0 {
		t.Fatalf("countByLabel(dog) = %d, want 0 (no active track of that label)", got)
	}
}

func TestNewTrackManager_FilterSpecErrorPropagates(t *testing.T) {
	_, err := newTrackManager(&UseCase{}, dto.RecognitionRequest{Filter: "unicorn"})
	if err == nil {
		t.Fatal("newTrackManager with an unknown label should return an error, got nil")
	}
}

func TestNewTrackManager_ValidFilterBuildsTermsMap(t *testing.T) {
	m, err := newTrackManager(&UseCase{}, dto.RecognitionRequest{Filter: "person*2,car"})
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if len(m.filterTerms) != 2 {
		t.Fatalf("filterTerms = %+v, want 2 entries", m.filterTerms)
	}
	if m.filterTerms["person"] != 2 {
		t.Fatalf(`filterTerms["person"] = %d, want 2`, m.filterTerms["person"])
	}
	if m.filterTerms["car"] != 1 {
		t.Fatalf(`filterTerms["car"] = %d, want 1`, m.filterTerms["car"])
	}
}

func TestNewTrackManager_EmptyFilterMeansNoGate(t *testing.T) {
	m, err := newTrackManager(&UseCase{}, dto.RecognitionRequest{Filter: ""})
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if len(m.filterTerms) != 0 {
		t.Fatalf("filterTerms = %+v, want empty (no filter requested)", m.filterTerms)
	}
	if !m.acceptsLabel("anything") {
		t.Fatal("acceptsLabel should accept every label when no filter was requested")
	}
}
