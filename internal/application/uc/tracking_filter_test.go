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

// mockSemanticEncoder: EncodeText returns a fixed reference direction per
// text — {1,0,0} by default (a term's own compound text, e.g. "person with
// a red hat"), or {0,1,0} for any text listed in baseAxisTexts (a term's
// LabelHint/base noun, e.g. "person" — needed for differential scoring,
// defaultDifferentialMargin, docs/adr/clip-backend.md § 23-24). EncodeImage
// looks up the crop's "WxH" (derived from the cropped frame's own Bounds(),
// which frame.Crop zeroes to box's width/height) in scoreByCropSize /
// baseScoreByCropSize to decide the cosine similarity it should produce
// against each of those two reference axes independently — lets tests pick
// an exact compound score AND an exact base-noun score per candidate box
// without reimplementing real CLIP math. A crop size not present in
// scoreByCropSize scores 0 against both axes (orthogonal — "no match") by
// default.
//
// IMPORTANT for any filter whose semantic term has a LabelHint (i.e. its
// free text mentions a COCO word, e.g. "person" in "person with a red
// hat"): that word MUST be listed in baseAxisTexts. Without it, EncodeText
// returns the *same* {1,0,0} for both the compound term and its base noun
// (they're indistinguishable strings otherwise) — term.BaseEmbedding then
// equals term.Embedding, baseScore always equals score, delta is always 0,
// and the differential gate (defaultDifferentialMargin) rejects every
// candidate regardless of scoreByCropSize. Listing the word in
// baseAxisTexts moves the base noun onto its own axis, so baseScore falls
// back to baseScoreByCropSize's default of 0 — reproducing pre-differential
// behavior (score >= defaultSimilarityThreshold alone decides, since 0.20 >
// 0.02 makes the margin check trivially true whenever the threshold check
// already passed) unless the test explicitly sets baseScoreByCropSize too.
type mockSemanticEncoder struct {
	scoreByCropSize     map[string]float32
	baseScoreByCropSize map[string]float32
	baseAxisTexts       map[string]bool
	encodeTextErr       error
	encodeImageErr      error
	encodeTextCalls     int
}

func (m *mockSemanticEncoder) EncodeText(text string) (entities.Embedding, error) {
	m.encodeTextCalls++
	if m.encodeTextErr != nil {
		return nil, m.encodeTextErr
	}
	if m.baseAxisTexts[text] {
		return entities.Embedding{0, 1, 0}, nil
	}
	return entities.Embedding{1, 0, 0}, nil
}

func (m *mockSemanticEncoder) EncodeImage(frame *entities.Frame) (entities.Embedding, error) {
	if m.encodeImageErr != nil {
		return nil, m.encodeImageErr
	}
	b := frame.Image.Bounds()
	key := fmt.Sprintf("%dx%d", b.Dx(), b.Dy())
	compound, ok := m.scoreByCropSize[key]
	if !ok {
		return entities.Embedding{0, 0, 1}, nil
	}
	base := m.baseScoreByCropSize[key]
	remainderSq := 1 - compound*compound - base*base
	if remainderSq < 0 {
		remainderSq = 0
	}
	return entities.Embedding{compound, base, float32(math.Sqrt(float64(remainderSq)))}, nil
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
		gallery:         NewReferenceGallery(),
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
	// 2, not 1: "person with a red hat" mentions "person" (a COCO word) ->
	// semanticLabelHint finds it -> newTrackManager also encodes "person"
	// alone for BaseEmbedding (differential scoring, § 23-24) — both calls
	// happen once here, still not per frame.
	if encoder.encodeTextCalls != 2 {
		t.Fatalf("EncodeText called %d times, want exactly 2 (compound term + base noun, both once at newTrackManager, not per frame)", encoder.encodeTextCalls)
	}
	if got.BaseEmbedding == nil {
		t.Fatal("semantic term with a LabelHint should have a non-nil BaseEmbedding")
	}
}

// --- gallery-backed filter terms (TODO.md § D/§ H1, docs/adr/clip-backend.md § 24) ---

func TestNewTrackManager_GalleryTerm_UsesGalleryEmbeddingNotEncodeText(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})

	galleryEmbedding := entities.Embedding{0.5, 0.5, 0}
	if err := uc.gallery.Add("mon_sac", galleryEmbedding); err != nil {
		t.Fatalf("gallery.Add() error = %v", err)
	}

	m, err := newTrackManager(uc, dto.RecognitionRequest{Filter: "mon_sac*1"})
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	got, ok := m.terms["mon_sac"]
	if !ok {
		t.Fatalf("terms = %+v, missing the gallery term", m.terms)
	}
	if len(got.Embedding) != len(galleryEmbedding) || got.Embedding[0] != galleryEmbedding[0] {
		t.Fatalf("term.Embedding = %v, want the gallery's own embedding %v", got.Embedding, galleryEmbedding)
	}
	if encoder.encodeTextCalls != 0 {
		t.Fatalf("EncodeText called %d times, want 0 (a registered gallery name should never fall back to text encoding)", encoder.encodeTextCalls)
	}
}

func TestReanchor_GalleryTerm_MatchesCandidateByImageSimilarity(t *testing.T) {
	target := boxSized("backpack", 0, 40, 40)
	encoder := &mockSemanticEncoder{scoreByCropSize: map[string]float32{cropSizeKey(target): 0.9}}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{target}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	// Same reference axis {1,0} the mock's EncodeText/EncodeImage already
	// use by default — a real gallery embedding would come from a real
	// EncodeImage call on a reference crop, not from EncodeText, but the
	// mock's scoring math only cares about the axis, not its origin.
	if err := uc.gallery.Add("mon_sac", entities.Embedding{1, 0, 0}); err != nil {
		t.Fatalf("gallery.Add() error = %v", err)
	}

	req := dto.RecognitionRequest{Filter: "mon_sac*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (candidate scores 0.9 against the gallery embedding)", got)
	}
	for _, obj := range m.active {
		if obj.filterKey != "mon_sac" {
			t.Fatalf("filterKey = %q, want %q", obj.filterKey, "mon_sac")
		}
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

	encoder := &mockSemanticEncoder{
		scoreByCropSize: map[string]float32{
			cropSizeKey(best):           0.5,
			cropSizeKey(middle):         0.4,
			cropSizeKey(belowThreshold): 0.1,
		},
		// "person with a red hat" mentions "person" -> LabelHint -> a
		// BaseEmbedding is encoded too (differential scoring, § 23-24).
		// Mapping it onto its own axis (rather than the default, which
		// mirrors the compound term's own axis) keeps baseScore at 0 here
		// (baseScoreByCropSize unset) — same effective behavior as before
		// differential scoring existed, see mockSemanticEncoder's doc
		// comment.
		baseAxisTexts: map[string]bool{"person": true},
	}
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
	// reanchor's doc comment / TODO.md § A: an exact term and a semantic
	// term sharing the *same* YOLO label is the deliberate no-overlap
	// default (pass 1 claims every box with that label first, cap or
	// not), confirmed by the user 2026-08-11, not tested here on purpose.
	car := box("car", 0)                       // claimed by the exact "car" term
	hatPerson := boxSized("person", 1, 40, 40) // "person" isn't requested as an exact term here, so pass 1 never touches it — scored by the semantic term in pass 2

	encoder := &mockSemanticEncoder{
		scoreByCropSize: map[string]float32{cropSizeKey(hatPerson): 0.9},
		// "person with sunglasses" mentions "person" -> LabelHint -> see
		// TestReanchor_SemanticTerm_RanksAndCaps' comment on the same
		// pattern.
		baseAxisTexts: map[string]bool{"person": true},
	}
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

// TestReanchor_SemanticTerm_DifferentialMarginRejectsBareBaseNoun
// reproduces the real bug reported live (docs/adr/clip-backend.md § 23):
// "person*1,person with a hat*1+overlap" drew two boxes whether or not the
// person actually wore a hat, because a bare "person" already scores above
// defaultSimilarityThreshold on its own (~0.235-0.238, § 10) — an absolute
// threshold alone can't tell the compound term's score apart from the base
// noun's own score. Here the crop scores 0.27 for "person with a hat"
// (clears defaultSimilarityThreshold) but *also* 0.26 for "person" alone
// (LabelHint's BaseEmbedding) — delta 0.01, under defaultDifferentialMargin
// (0.02) — the differential gate must reject it despite the absolute
// threshold passing.
func TestReanchor_SemanticTerm_DifferentialMarginRejectsBareBaseNoun(t *testing.T) {
	noHat := boxSized("person", 0, 40, 80)
	encoder := &mockSemanticEncoder{
		scoreByCropSize:     map[string]float32{cropSizeKey(noHat): 0.27},
		baseScoreByCropSize: map[string]float32{cropSizeKey(noHat): 0.26},
		baseAxisTexts:       map[string]bool{"person": true},
	}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{noHat}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person with a hat*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 0 {
		t.Fatalf("active tracks = %d, want 0 (score clears the absolute threshold but the delta over the bare \"person\" score is under defaultDifferentialMargin — no real signal that a hat is actually present)", got)
	}
}

// TestReanchor_SemanticTerm_DifferentialMarginAcceptsRealSignal is the
// positive counterpart: same absolute score (0.27) but a distinctly lower
// base-noun score (0.20, delta 0.07 >= defaultDifferentialMargin) — a real
// hat should still match normally, the differential gate isn't a blanket
// rejection of every compound term.
func TestReanchor_SemanticTerm_DifferentialMarginAcceptsRealSignal(t *testing.T) {
	withHat := boxSized("person", 0, 40, 80)
	encoder := &mockSemanticEncoder{
		scoreByCropSize:     map[string]float32{cropSizeKey(withHat): 0.27},
		baseScoreByCropSize: map[string]float32{cropSizeKey(withHat): 0.20},
		baseAxisTexts:       map[string]bool{"person": true},
	}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{withHat}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person with a hat*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (real margin over the base noun's own score should still match)", got)
	}
}

func TestReanchor_ExactAndSemanticSameLabel_DefaultNoOverlap_OnlyExactMatches(t *testing.T) {
	target := boxSized("person", 0, 40, 40)
	encoder := &mockSemanticEncoder{scoreByCropSize: map[string]float32{
		cropSizeKey(target): 0.9, // would clearly match if ever evaluated
	}}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{target}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	// "person with a red hat" mentions "person" (semanticLabelHint would
	// restrict it to "person"-labeled boxes) — irrelevant here since the
	// default (no +overlap) must already exclude this box at the claimed
	// check, before the label hint is even consulted.
	req := dto.RecognitionRequest{Filter: "person*1,person with a red hat*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (only the exact term should match — no +overlap on the semantic term)", got)
	}
	for _, obj := range m.active {
		if obj.filterKey != "person" {
			t.Fatalf("filterKey = %q, want %q (the exact term, not the semantic one)", obj.filterKey, "person")
		}
	}
}

func TestReanchor_ExactAndSemanticSameLabel_WithOverlap_BothMatch(t *testing.T) {
	target := boxSized("person", 0, 40, 40)
	encoder := &mockSemanticEncoder{
		scoreByCropSize: map[string]float32{cropSizeKey(target): 0.9},
		baseAxisTexts:   map[string]bool{"person": true},
	}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{target}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person*1,person with a red hat*1+overlap"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 2 {
		t.Fatalf("active tracks = %d, want 2 (+overlap lets the semantic term also claim the box the exact term already claimed)", got)
	}

	gotKeys := map[string]bool{}
	for _, obj := range m.active {
		gotKeys[obj.filterKey] = true
	}
	for _, want := range []string{"person", "person with a red hat"} {
		if !gotKeys[want] {
			t.Fatalf("missing track with filterKey %q among active tracks %v", want, gotKeys)
		}
	}
}

func TestReanchor_SemanticTerm_LabelHintIgnoresMismatchedHighScorer(t *testing.T) {
	// "person with a yellow hat" mentions "person" -> LabelHint="person".
	// The couch scores far higher than the person, but must still be
	// ignored — this is exactly the real-world case that motivated
	// LabelHint (docs/adr/clip-backend.md § 17: a couch/potted plant
	// outscoring the actual person for a "person ..." query).
	person := boxSized("person", 0, 40, 40)
	couch := boxSized("couch", 1, 90, 90)

	encoder := &mockSemanticEncoder{
		scoreByCropSize: map[string]float32{
			cropSizeKey(person): 0.25,
			cropSizeKey(couch):  0.90,
		},
		baseAxisTexts: map[string]bool{"person": true},
	}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{couch, person}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person with a yellow hat*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if got := m.terms["person with a yellow hat"].LabelHint; got != "person" {
		t.Fatalf("LabelHint = %q, want %q", got, "person")
	}

	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (the couch must never be a candidate)", got)
	}
	for _, obj := range m.active {
		if obj.track.Class != "person" {
			t.Fatalf("matched track's class = %q, want %q (the couch outscored it but should have been excluded by LabelHint)", obj.track.Class, "person")
		}
	}
}

// TestReanchor_SpawnedTrackConfirmsInExactlyMinHitsCycles is a regression
// test for a bug found 2026-08-11 while wiring +overlap (not specific to
// it — affects every spawn, single-term filters included): spawn() didn't
// register the new track's ID into matchedTrackIDs, so missUnmatched
// (called once per reanchor cycle, after every term) would call Miss() on
// a track in the very same cycle it was born — resetting its hit streak
// back to 0 immediately. A persistently-matching box took one extra cycle
// to reach StateConfirmed (4 instead of minHitsToConfirm=3), and in the
// worst case (a brief miss on the cycle right after spawning) could reach
// StateLost without ever confirming, since maxMissesBeforeLost=2.
// --- relational term tests ("container%+%attachment", docs/adr/clip-backend.md § 24) ---

// containedBackpack/farBackpack/personContainer are shared by the
// relational tests below: a "person" box, one "backpack" box fully inside
// it (containmentRatio 1.0), and one far away (containmentRatio 0).
func personContainer() entities.BoundingBox {
	return entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 100, Y2: 200}
}
func containedBackpack() entities.BoundingBox {
	return entities.BoundingBox{Label: "backpack", X1: 20, Y1: 20, X2: 80, Y2: 80}
}
func farBackpack() entities.BoundingBox {
	return entities.BoundingBox{Label: "backpack", X1: 1000, Y1: 1000, X2: 1060, Y2: 1060}
}

func TestReanchor_RelationalTerm_ContainmentMatches(t *testing.T) {
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{personContainer(), containedBackpack()}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%+%backpack"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (backpack fully inside person satisfies containment)", got)
	}
	for _, obj := range m.active {
		if obj.filterKey != "person%+%backpack" {
			t.Fatalf("filterKey = %q, want %q", obj.filterKey, "person%+%backpack")
		}
		if obj.track.Class != "person" {
			t.Fatalf("track.Class = %q, want %q (the container box, not the attachment)", obj.track.Class, "person")
		}
	}
}

func TestReanchor_RelationalTerm_NoContainment_NoMatch(t *testing.T) {
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{personContainer(), farBackpack()}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%+%backpack"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 0 {
		t.Fatalf("active tracks = %d, want 0 (backpack far from person never satisfies containment)", got)
	}
}

func TestReanchor_RelationalTerm_CapLimitsPairs(t *testing.T) {
	personA := entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 100, Y2: 200}
	backpackA := entities.BoundingBox{Label: "backpack", X1: 20, Y1: 20, X2: 80, Y2: 80}
	personB := entities.BoundingBox{Label: "person", X1: 1000, Y1: 0, X2: 1100, Y2: 200}
	backpackB := entities.BoundingBox{Label: "backpack", X1: 1020, Y1: 20, X2: 1080, Y2: 80}

	detector := &mockObjectDetector{boxes: []entities.BoundingBox{personA, backpackA, personB, backpackB}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%+%backpack*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (cap=1 keeps only one of the two equally-valid pairs)", got)
	}
}

// TestReanchor_RelationalTerm_GreedyOneToOne: one container box, two
// attachment boxes both individually satisfying containment — the
// container can only join ONE pair per cycle (default cardinality,
// docs/adr/clip-backend.md § 24: "+shared" for N:M is deferred, not
// implemented), even with a cap generous enough to allow two.
func TestReanchor_RelationalTerm_GreedyOneToOne(t *testing.T) {
	container := entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 200, Y2: 200}
	attachment1 := entities.BoundingBox{Label: "backpack", X1: 10, Y1: 10, X2: 60, Y2: 60}
	attachment2 := entities.BoundingBox{Label: "backpack", X1: 100, Y1: 100, X2: 150, Y2: 150}

	detector := &mockObjectDetector{boxes: []entities.BoundingBox{container, attachment1, attachment2}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%+%backpack*2"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (one container box can only join one pair per cycle by default, despite cap=2 and two valid attachments)", got)
	}
}

// TestReanchor_RelationalTerm_SharedAllowsSameAttachmentInMultiplePairs
// is GreedyOneToOne's counterpart with "+shared": one backpack box fully
// inside *two* overlapping person boxes (the classic "unattended bag near
// several people" case, docs/adr/clip-backend.md § 24) — without +shared
// only one (container, attachment) pair could be kept per cycle; with it,
// both containers get their own track since the shared side here is the
// *attachment*, and the container (the tracked entity) genuinely differs
// between the two pairs.
func TestReanchor_RelationalTerm_SharedAllowsSameAttachmentInMultiplePairs(t *testing.T) {
	personA := entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 100, Y2: 100}
	personB := entities.BoundingBox{Label: "person", X1: 50, Y1: 0, X2: 150, Y2: 100}   // overlaps personA
	backpack := entities.BoundingBox{Label: "backpack", X1: 60, Y1: 10, X2: 90, Y2: 90} // fully inside both

	detector := &mockObjectDetector{boxes: []entities.BoundingBox{personA, personB, backpack}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%+%backpack*2+shared"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 2 {
		t.Fatalf("active tracks = %d, want 2 (+shared lets the same backpack satisfy both person pairs)", got)
	}
	gotX1 := map[float32]bool{}
	for _, obj := range m.active {
		gotX1[obj.track.LastBox().X1] = true
	}
	for _, want := range []float32{personA.X1, personB.X1} {
		if !gotX1[want] {
			t.Fatalf("missing a track for the person at X1=%v among active tracks %v", want, gotX1)
		}
	}
}

// TestReanchor_RelationalTerm_SharedFalseByDefault re-runs the same scene
// as the +shared test above but *without* the option — confirms the
// default really is exclusive (this is GreedyOneToOne's scenario in
// reverse: shared attachment instead of shared container, to make sure
// the default guard catches both directions, not just the one
// GreedyOneToOne already covers).
func TestReanchor_RelationalTerm_SharedFalseByDefault(t *testing.T) {
	personA := entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 100, Y2: 100}
	personB := entities.BoundingBox{Label: "person", X1: 50, Y1: 0, X2: 150, Y2: 100}
	backpack := entities.BoundingBox{Label: "backpack", X1: 60, Y1: 10, X2: 90, Y2: 90}

	detector := &mockObjectDetector{boxes: []entities.BoundingBox{personA, personB, backpack}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%+%backpack*2"} // no +shared
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (without +shared, the backpack can only join one pair despite cap=2)", got)
	}
}

func TestReanchor_RelationalTerm_NearMatchesWithinDistance(t *testing.T) {
	person := entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 50, Y2: 100}
	car := entities.BoundingBox{Label: "car", X1: 70, Y1: 0, X2: 150, Y2: 80} // 20px gap on the X axis

	detector := &mockObjectDetector{boxes: []entities.BoundingBox{person, car}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%near=30%car"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1 (20px gap is within the 30px near threshold)", got)
	}
}

func TestReanchor_RelationalTerm_NearRejectsBeyondDistance(t *testing.T) {
	person := entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 50, Y2: 100}
	car := entities.BoundingBox{Label: "car", X1: 200, Y1: 0, X2: 280, Y2: 80} // 150px gap

	detector := &mockObjectDetector{boxes: []entities.BoundingBox{person, car}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%near=30%car"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	if got := m.count(); got != 0 {
		t.Fatalf("active tracks = %d, want 0 (150px gap exceeds the 30px near threshold)", got)
	}
}

// TestReanchor_RelationalTerm_NearRanksClosestFirst uses two independent
// (person, car) pairs — a container's own box is what's tracked (see
// TestReanchor_RelationalTerm_ContainmentMatches), so verifying ranking
// specifically requires an observable difference *between pairs*, not
// just within one: personA/carA are 5px apart, personB/carB 20px apart,
// both under the 30px threshold, but cap=1 can only keep one pair — it
// must be the closer one, not whichever happened to be scored first.
func TestReanchor_RelationalTerm_NearRanksClosestFirst(t *testing.T) {
	personA := entities.BoundingBox{Label: "person", X1: 0, Y1: 0, X2: 50, Y2: 100}
	carA := entities.BoundingBox{Label: "car", X1: 55, Y1: 0, X2: 120, Y2: 80} // 5px gap

	personB := entities.BoundingBox{Label: "person", X1: 1000, Y1: 0, X2: 1050, Y2: 100}
	carB := entities.BoundingBox{Label: "car", X1: 1070, Y1: 0, X2: 1140, Y2: 80} // 20px gap

	// Deliberately not sorted — ranking must not depend on detection order.
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{carB, personB, carA, personA}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person%near=30%car*1"}
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
		if obj.track.LastBox().X1 != personA.X1 {
			t.Fatalf("kept track's box = %+v, want personA's box (the closer pair, 5px vs 20px gap)", obj.track.LastBox())
		}
	}
}

func TestReanchor_SpawnedTrackConfirmsInExactlyMinHitsCycles(t *testing.T) {
	target := box("person", 0)
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{target}}
	uc := newTestUseCase(detector, &mockSemanticEncoder{}, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	const minHitsToConfirm = 3 // entities.Track's own unexported constant, mirrored here
	for cycle := 1; cycle <= minHitsToConfirm; cycle++ {
		if err := m.reanchor(testFrame(), req); err != nil {
			t.Fatalf("reanchor() cycle %d error = %v", cycle, err)
		}
	}

	if got := m.count(); got != 1 {
		t.Fatalf("active tracks = %d, want 1", got)
	}
	for _, obj := range m.active {
		if obj.track.State != entities.StateConfirmed {
			t.Fatalf("track state after %d matching cycles = %s, want %s (a same-cycle phantom Miss() on spawn would delay this by at least one extra cycle)",
				minHitsToConfirm, obj.track.State, entities.StateConfirmed)
		}
	}
}

// TestBoxes_DistinguishesTracksSharingTheSamePhysicalBox is a regression
// test for docs/adr/clip-backend.md § 18: two tracks matched to the same
// physical detection (an exact term + a "+overlap" semantic term) share
// the same underlying YOLO box.Label ("person" for both) — drawing code
// must key color/label off FilterKey instead, which requires boxes() to
// actually return it.
func TestBoxes_DistinguishesTracksSharingTheSamePhysicalBox(t *testing.T) {
	target := boxSized("person", 0, 40, 40)
	encoder := &mockSemanticEncoder{
		scoreByCropSize: map[string]float32{cropSizeKey(target): 0.9},
		baseAxisTexts:   map[string]bool{"person": true},
	}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{target}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person*1,person with a red hat*1+overlap"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	boxes := m.boxes()
	if len(boxes) != 2 {
		t.Fatalf("boxes() returned %d entries, want 2", len(boxes))
	}

	gotKeys := map[string]bool{}
	for _, b := range boxes {
		if b.Box.Label != "person" {
			t.Fatalf("Box.Label = %q, want %q (YOLO's own label, same for both tracks)", b.Box.Label, "person")
		}
		if b.TrackID == "" {
			t.Fatal("TrackID is empty, want a real track ID")
		}
		gotKeys[b.FilterKey] = true
	}
	for _, want := range []string{"person", "person with a red hat"} {
		if !gotKeys[want] {
			t.Fatalf("missing FilterKey %q among boxes() results %v — drawing can't distinguish the two tracks without it", want, gotKeys)
		}
	}
}

func TestCascadeOffsets(t *testing.T) {
	sameBox := entities.BoundingBox{X1: 0, Y1: 0, X2: 100, Y2: 100}
	nearbyBox := entities.BoundingBox{X1: 500, Y1: 500, X2: 600, Y2: 600} // no overlap at all with sameBox

	t.Run("single box gets no offset", func(t *testing.T) {
		boxes := []trackedBox{{Box: sameBox, TrackID: "track-1"}}
		got := cascadeOffsets(boxes)
		if len(got) != 1 || got[0] != 0 {
			t.Fatalf("cascadeOffsets = %v, want [0]", got)
		}
	})

	t.Run("non-overlapping boxes get no offset", func(t *testing.T) {
		boxes := []trackedBox{{Box: sameBox, TrackID: "track-1"}, {Box: nearbyBox, TrackID: "track-2"}}
		got := cascadeOffsets(boxes)
		for i, off := range got {
			if off != 0 {
				t.Fatalf("offset[%d] = %v, want 0 (no overlap)", i, off)
			}
		}
	})

	t.Run("two identical boxes stagger by TrackID order", func(t *testing.T) {
		boxes := []trackedBox{{Box: sameBox, TrackID: "track-2"}, {Box: sameBox, TrackID: "track-1"}}
		got := cascadeOffsets(boxes)
		// track-1 sorts before track-2 regardless of slice position, so
		// track-1 (index 1 here) must be the one with offset 0.
		if got[1] != 0 {
			t.Fatalf("offset for track-1 = %v, want 0 (first in TrackID order)", got[1])
		}
		if got[0] != cascadeStepPx {
			t.Fatalf("offset for track-2 = %v, want %v (second in TrackID order)", got[0], cascadeStepPx)
		}
	})

	t.Run("three identical boxes stagger incrementally", func(t *testing.T) {
		boxes := []trackedBox{{Box: sameBox, TrackID: "track-a"}, {Box: sameBox, TrackID: "track-b"}, {Box: sameBox, TrackID: "track-c"}}
		got := cascadeOffsets(boxes)
		want := []float32{0, cascadeStepPx, 2 * cascadeStepPx}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("offset[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("stable regardless of input slice order", func(t *testing.T) {
		a := []trackedBox{{Box: sameBox, TrackID: "track-1"}, {Box: sameBox, TrackID: "track-2"}}
		b := []trackedBox{{Box: sameBox, TrackID: "track-2"}, {Box: sameBox, TrackID: "track-1"}}

		offA := cascadeOffsets(a)
		offB := cascadeOffsets(b)

		byTrack := func(boxes []trackedBox, offs []float32, id string) float32 {
			for i, tb := range boxes {
				if tb.TrackID == id {
					return offs[i]
				}
			}
			t.Fatalf("track %q not found", id)
			return -1
		}

		if byTrack(a, offA, "track-1") != byTrack(b, offB, "track-1") {
			t.Fatal("track-1's offset changed depending on input slice order — must be stable across frames")
		}
		if byTrack(a, offA, "track-2") != byTrack(b, offB, "track-2") {
			t.Fatal("track-2's offset changed depending on input slice order — must be stable across frames")
		}
	})
}

// TestBoxes_ScoreReflectsWhatDecidedTheMatch is a regression test for
// docs/adr/clip-backend.md § 20: the box drawn for a semantic-term track
// used to show YOLO's own box.Confidence (unrelated to CLIP, and
// typically far higher-looking than the real CLIP margin) instead of the
// score that actually decided the match. Exact-term tracks have no CLIP
// score at all — they should keep reporting 0 (display code falls back to
// Box.Confidence in that case).
func TestBoxes_ScoreReflectsWhatDecidedTheMatch(t *testing.T) {
	exactTarget := box("car", 0)
	semanticTarget := boxSized("person", 1, 40, 40)

	encoder := &mockSemanticEncoder{
		scoreByCropSize: map[string]float32{cropSizeKey(semanticTarget): 0.27},
		baseAxisTexts:   map[string]bool{"person": true},
	}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{exactTarget, semanticTarget}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "car,person with a red hat*1"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}
	if err := m.reanchor(testFrame(), req); err != nil {
		t.Fatalf("reanchor error = %v", err)
	}

	scoreByKey := map[string]float32{}
	for _, b := range m.boxes() {
		scoreByKey[b.FilterKey] = b.Score
	}

	if got := scoreByKey["car"]; got != 0 {
		t.Fatalf(`boxes()["car"].Score = %v, want 0 (exact term, no CLIP score)`, got)
	}
	if got := scoreByKey["person with a red hat"]; got != 0.27 {
		t.Fatalf(`boxes()["person with a red hat"].Score = %v, want 0.27 (the CLIP score that decided the match)`, got)
	}
}

// TestReanchor_OverlapTracks_ScoreStaysWithCorrectTrackAcrossCycles is a
// regression test for docs/adr/clip-backend.md § 21: a real bug (not
// hypothetical, found from a user screenshot) where an exact term and a
// "+overlap" semantic term matched to the same physical object — same
// track.Class ("person" for both, since both come from the same YOLO
// box.Label) — could have their re-anchor calls land on *either* track,
// because bestMatch only checked Class, not filterKey. The single-cycle
// tests above (e.g. TestReanchor_ExactAndSemanticSameLabel_WithOverlap_
// BothMatch) never caught this: on a first cycle m.active starts empty,
// so both terms go through spawn(), never bestMatch() — the bug only
// shows up from the *second* cycle onward, once both tracks already exist
// and reanchor has to choose which one to re-anchor.
func TestReanchor_OverlapTracks_ScoreStaysWithCorrectTrackAcrossCycles(t *testing.T) {
	target := boxSized("person", 0, 40, 40)
	const semanticScore = float32(0.21)

	encoder := &mockSemanticEncoder{
		scoreByCropSize: map[string]float32{cropSizeKey(target): semanticScore},
		baseAxisTexts:   map[string]bool{"person": true},
	}
	detector := &mockObjectDetector{boxes: []entities.BoundingBox{target}}
	uc := newTestUseCase(detector, encoder, &mockAlertSender{})

	req := dto.RecognitionRequest{Filter: "person*1,person with a yellow hat*1+overlap"}
	m, err := newTrackManager(uc, req)
	if err != nil {
		t.Fatalf("newTrackManager error = %v", err)
	}

	// Run several cycles — the bug (map iteration randomizing which call's
	// bestMatch lands on which track) doesn't necessarily reproduce on the
	// very first post-spawn cycle every single run, so check every cycle,
	// not just the last.
	for cycle := 1; cycle <= 5; cycle++ {
		if err := m.reanchor(testFrame(), req); err != nil {
			t.Fatalf("reanchor() cycle %d error = %v", cycle, err)
		}

		if got := m.count(); got != 2 {
			t.Fatalf("cycle %d: active tracks = %d, want 2", cycle, got)
		}

		scoreByKey := map[string]float32{}
		for _, b := range m.boxes() {
			scoreByKey[b.FilterKey] = b.Score
		}

		if got, ok := scoreByKey["person"]; !ok || got != 0 {
			t.Fatalf(`cycle %d: boxes()["person"].Score = %v, want 0 (exact term, no CLIP score — must never leak the semantic term's score)`, cycle, got)
		}
		if got, ok := scoreByKey["person with a yellow hat"]; !ok || got != semanticScore {
			t.Fatalf(`cycle %d: boxes()["person with a yellow hat"].Score = %v, want %v (must never be overwritten by the exact term's call)`, cycle, got, semanticScore)
		}
	}
}
