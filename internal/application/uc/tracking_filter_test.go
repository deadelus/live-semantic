package uc

import (
	"errors"
	"fmt"
	"image"
	"math"
	"testing"
	"time"

	"live-semantic/internal/application/dto"
	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/inference"
	"live-semantic/internal/infrastructure/tracking"

	"github.com/deadelus/go-clean-app/v2/logger"
)

// testFrame returns a Frame backed by a real (if blank) image, large
// enough to contain every synthetic test box below without clipping —
// needed because reanchor's semantic pass calls frame.Crop(box), which
// panics on a Frame with a nil Image (frame.Image.Bounds()).
func testFrame() *entities.Frame {
	return &entities.Frame{Image: image.NewRGBA(image.Rect(0, 0, 5000, 5000))}
}

// --- test doubles -----------------------------------------------------
//
// Package-local (not shared with e.g. internal/transport/adapters/api's
// mocks, which live in a different package) — small enough not to be
// worth a shared test-helpers package for now.

type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Close()               {}

var _ logger.Logger = noopLogger{}

type mockObjectDetector struct {
	boxes []entities.BoundingBox
	err   error
}

func (m *mockObjectDetector) AnalyzeFrame(frame *entities.Frame) (*inference.DetectionResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &inference.DetectionResult{Frame: frame, BoundingBoxes: m.boxes}, nil
}
func (m *mockObjectDetector) Cleanup() {}

// mockSemanticEncoder: EncodeText always returns a fixed reference
// direction ({1,0}); EncodeImage looks up the crop's "WxH" (derived from
// the cropped frame's own Bounds(), which frame.Crop zeroes to box's
// width/height) in scoreByCropSize to decide which cosine similarity it
// should produce against that reference — lets tests pick an exact score
// per candidate box without reimplementing real CLIP math. A crop size not
// present in the map scores 0 (orthogonal — "no match") by default.
type mockSemanticEncoder struct {
	scoreByCropSize map[string]float32
	encodeTextErr   error
	encodeImageErr  error
	encodeTextCalls int
}

func (m *mockSemanticEncoder) EncodeText(string) (entities.Embedding, error) {
	m.encodeTextCalls++
	if m.encodeTextErr != nil {
		return nil, m.encodeTextErr
	}
	return entities.Embedding{1, 0}, nil
}

func (m *mockSemanticEncoder) EncodeImage(frame *entities.Frame) (entities.Embedding, error) {
	if m.encodeImageErr != nil {
		return nil, m.encodeImageErr
	}
	b := frame.Image.Bounds()
	key := fmt.Sprintf("%dx%d", b.Dx(), b.Dy())
	score, ok := m.scoreByCropSize[key]
	if !ok {
		return entities.Embedding{0, 1}, nil
	}
	return entities.Embedding{score, float32(math.Sqrt(float64(1 - score*score)))}, nil
}

func (m *mockSemanticEncoder) Cleanup() {}

type mockObjectTracker struct{}

func (mockObjectTracker) Init(*entities.Frame, entities.BoundingBox) error { return nil }
func (mockObjectTracker) Update(*entities.Frame) (entities.BoundingBox, bool) {
	return entities.BoundingBox{}, true
}
func (mockObjectTracker) Cleanup() {}

func mockTrackerFactory() (tracking.ObjectTracker, error) { return mockObjectTracker{}, nil }

type mockAlertSender struct {
	notifications []entities.Message
}

func (m *mockAlertSender) Notify(msg entities.Message) error {
	m.notifications = append(m.notifications, msg)
	return nil
}
func (m *mockAlertSender) Cleanup() {}

func newTestUseCase(detector *mockObjectDetector, encoder *mockSemanticEncoder, notifier *mockAlertSender) *UseCase {
	return &UseCase{
		logger:          noopLogger{},
		objectDetector:  detector,
		semanticEncoder: encoder,
		notifier:        notifier,
		trackerFactory:  mockTrackerFactory,
	}
}

// box is a small helper for a test bounding box with a given label and a
// distinct, non-overlapping position derived from its index — keeps
// bestMatch's IoU association from accidentally merging two test boxes
// meant to be different objects.
func box(label string, index int) entities.BoundingBox {
	offset := float32(index * 1000)
	return entities.BoundingBox{Label: label, X1: offset, Y1: 0, X2: offset + 10, Y2: 10}
}

// boxSized additionally controls width/height, so a test can key
// mockSemanticEncoder.scoreByCropSize to a specific box.
func boxSized(label string, index int, w, h float32) entities.BoundingBox {
	offset := float32(index * 1000)
	return entities.BoundingBox{Label: label, X1: offset, Y1: 0, X2: offset + w, Y2: h}
}

func cropSizeKey(b entities.BoundingBox) string {
	return fmt.Sprintf("%dx%d", int(b.X2-b.X1), int(b.Y2-b.Y1))
}

// --- tests --------------------------------------------------------------

func TestCountByFilterKey(t *testing.T) {
	m := &trackManager{
		active: map[string]*trackedObject{
			"track-1": {track: entities.NewTrack("track-1", box("person", 0), time.Now()), filterKey: "person"},
			"track-2": {track: entities.NewTrack("track-2", box("person", 1), time.Now()), filterKey: "person"},
			"track-3": {track: entities.NewTrack("track-3", box("car", 2), time.Now()), filterKey: "car"},
		},
	}

	if got := m.countByFilterKey("person"); got != 2 {
		t.Fatalf("countByFilterKey(person) = %d, want 2", got)
	}
	if got := m.countByFilterKey("car"); got != 1 {
		t.Fatalf("countByFilterKey(car) = %d, want 1", got)
	}
	if got := m.countByFilterKey("dog"); got != 0 {
		t.Fatalf("countByFilterKey(dog) = %d, want 0", got)
	}
}

func TestNewTrackManager_ParseErrorPropagates(t *testing.T) {
	uc := newTestUseCase(&mockObjectDetector{}, &mockSemanticEncoder{}, &mockAlertSender{})
	_, err := newTrackManager(uc, dto.RecognitionRequest{Filter: "person,person"})
	if err == nil {
		t.Fatal("newTrackManager with a duplicate term should return an error, got nil")
	}
}

func TestNewTrackManager_SemanticEncodeErrorPropagates(t *testing.T) {
	uc := newTestUseCase(&mockObjectDetector{}, &mockSemanticEncoder{encodeTextErr: errors.New("boom")}, &mockAlertSender{})
	_, err := newTrackManager(uc, dto.RecognitionRequest{Filter: "unicorn"})
	if err == nil {
		t.Fatal("newTrackManager should propagate EncodeText's error for a semantic term")
	}
}

func TestNewTrackManager_ExactTermsHaveNoEmbedding(t *testing.T) {
	uc := newTestUseCase(&mockObjectDetector{}, &mockSemanticEncoder{}, &mockAlertSender{})
	m, err := newTrackManager(uc, dto.RecognitionRequest{Filter: "person*2,car"})
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if len(m.terms) != 2 {
		t.Fatalf("terms = %+v, want 2 entries", m.terms)
	}
	if got := m.terms["person"]; got.Cap != 2 || got.Embedding != nil {
		t.Fatalf(`terms["person"] = %+v, want Cap=2 Embedding=nil`, got)
	}
	if got := m.terms["car"]; got.Cap != 1 || got.Embedding != nil {
		t.Fatalf(`terms["car"] = %+v, want Cap=1 Embedding=nil`, got)
	}
}

func TestNewTrackManager_SemanticTermGetsEmbedding(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	m, err := newTrackManager(uc, dto.RecognitionRequest{Filter: "person with a red hat*1"})
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	got, ok := m.terms["person with a red hat"]
	if !ok {
		t.Fatalf("terms = %+v, missing the semantic term", m.terms)
	}
	if got.Embedding == nil {
		t.Fatal("semantic term should have a non-nil Embedding")
	}
	if encoder.encodeTextCalls != 1 {
		t.Fatalf("EncodeText called %d times, want exactly 1 (once at newTrackManager, not per frame)", encoder.encodeTextCalls)
	}
}

func TestReanchor_NoFilter_TracksEverythingNoClipCall(t *testing.T) {
	boxes := []entities.BoundingBox{box("person", 0), box("car", 1)}
	detector := &mockObjectDetector{boxes: boxes}
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	m, err := newTrackManager(uc, dto.RecognitionRequest{Filter: ""})
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), dto.RecognitionRequest{Filter: ""}); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 2 {
		t.Fatalf("active tracks = %d, want 2 (no filter tracks everything)", got)
	}
	if encoder.encodeTextCalls != 0 {
		t.Fatalf("EncodeText called %d times, want 0 (no filter means no CLIP at all)", encoder.encodeTextCalls)
	}
}

func TestReanchor_ExactTerm_CapBlocksExtraSpawn(t *testing.T) {
	boxes := []entities.BoundingBox{box("person", 0), box("person", 1), box("car", 2)}
	detector := &mockObjectDetector{boxes: boxes}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (cap=1, car not requested, second person dropped)", got)
	}
}

func TestReanchor_SemanticTerm_RanksAndCaps(t *testing.T) {
	// Three candidates, distinct sizes so mockSemanticEncoder can score
	// each independently: best (0.5), middle (0.4, below cap but above
	// threshold — should lose to best under cap=1), and below-threshold
	// (0.1, must never spawn regardless of cap).
	best := boxSized("person", 0, 50, 50)
	middle := boxSized("person", 1, 60, 60)
	belowThreshold := boxSized("person", 2, 70, 70)

	encoder := &mockSemanticEncoder{scoreByCropSize: map[string]float32{
		cropSizeKey(best):           0.5,
		cropSizeKey(middle):         0.4,
		cropSizeKey(belowThreshold): 0.1,
	}}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{middle, best, belowThreshold}} // deliberately not sorted
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person with a red hat*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (cap=1)", got)
	}
	for _, obj := range m.active {
		if obj.track.LastBox().X1 != best.X1 {
			t.Fatalf("surviving track's box = %+v, want the highest-scoring candidate (%+v)", obj.track.LastBox(), best)
		}
		if obj.filterKey != "person with a red hat" {
			t.Fatalf("filterKey = %q, want %q", obj.filterKey, "person with a red hat")
		}
	}
}

func TestReanchor_MixedExactAndSemanticTerms(t *testing.T) {
	// Deliberately different YOLO labels ("car" vs "person") — see
	// reanchor's doc comment / TODO.md § A for the known, unresolved
	// interaction when an exact term and a semantic term could both want
	// the *same* YOLO label (pass 1 claims every box with that label
	// first, cap or not, so a semantic term never gets a chance at one
	// pass 1 already dropped to its cap). Not what this test is about.
	car := box("car", 0)                       // claimed by the exact "car" term
	hatPerson := boxSized("person", 1, 40, 40) // "person" isn't requested as an exact term here, so pass 1 never touches it — scored by the semantic term in pass 2

	encoder := &mockSemanticEncoder{scoreByCropSize: map[string]float32{
		cropSizeKey(hatPerson): 0.9,
	}}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{car, hatPerson}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "car*1,person with sunglasses*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 2 {
		t.Fatalf("active tracks = %d, want 2 (one exact match, one semantic match)", got)
	}

	var gotKeys []string
	for _, obj := range m.active {
		gotKeys = append(gotKeys, obj.filterKey)
	}
	wantKeys := map[string]bool{"car": true, "person with sunglasses": true}
	for _, k := range gotKeys {
		if !wantKeys[k] {
			t.Fatalf("unexpected filterKey %q among active tracks %v", k, gotKeys)
		}
		delete(wantKeys, k)
	}
	if len(wantKeys) != 0 {
		t.Fatalf("missing expected filterKeys: %v (got %v)", wantKeys, gotKeys)
	}
}

func TestReanchor_SemanticTerm_BelowThresholdNeverSpawns(t *testing.T) {
	tooLow := boxSized("person", 0, 20, 20)
	encoder := &mockSemanticEncoder{scoreByCropSize: map[string]float32{
		cropSizeKey(tooLow): 0.05, // well under defaultSimilarityThreshold (0.20)
	}}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{tooLow}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person with a red hat*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 0 {
		t.Fatalf("active tracks = %d, want 0 (score below threshold)", got)
	}
}
