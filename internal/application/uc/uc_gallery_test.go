package uc

import (
	"context"
	"errors"
	"image"
	"testing"
)

func TestUseCase_AddGalleryReference_EncodesAndStores(t *testing.T) {
	encoder := &mockSemanticEncoder{scoreByCropSize: map[string]float32{"20x20": 0.42}}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})

	crop := image.NewRGBA(image.Rect(0, 0, 20, 20))
	if err := uc.AddGalleryReference(context.Background(), "mon_sac", crop); err != nil {
		t.Fatalf("AddGalleryReference() error = %v", err)
	}

	if _, ok := uc.gallery.Get("mon_sac"); !ok {
		t.Fatal("gallery entry not found after AddGalleryReference()")
	}
}

// TestUseCase_AddGalleryReference_CocoCollisionRejected moved here from
// the old application/uc/gallery_test.go (2026-08-12 port extraction,
// see infrastructure/storage.GalleryStorage's doc comment): COCO-collision
// is a business rule the use case validates before ever reaching the
// Repository, which no longer knows what a COCO class is.
func TestUseCase_AddGalleryReference_CocoCollisionRejected(t *testing.T) {
	uc := newTestUseCase(&mockObjectDetector{}, &mockSemanticEncoder{}, &mockAlertSender{})
	err := uc.AddGalleryReference(context.Background(), "person", image.NewRGBA(image.Rect(0, 0, 10, 10)))
	if err == nil {
		t.Fatal("AddGalleryReference() with a name colliding with a COCO class should error — it would never be reachable as a filter term")
	}
}

// TestUseCase_RenameGalleryReference_CocoCollisionRejected — same rule,
// applied to the rename target.
func TestUseCase_RenameGalleryReference_CocoCollisionRejected(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	ctx := context.Background()
	if err := uc.AddGalleryReference(ctx, "thing", image.NewRGBA(image.Rect(0, 0, 10, 10))); err != nil {
		t.Fatalf("AddGalleryReference() error = %v", err)
	}
	if err := uc.RenameGalleryReference(ctx, "thing", "car"); err == nil {
		t.Fatal("RenameGalleryReference() onto a COCO class name should error")
	}
}

func TestUseCase_AddGalleryReference_PropagatesEncodeError(t *testing.T) {
	boom := errors.New("boom")
	encoder := &mockSemanticEncoder{encodeImageErr: boom}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})

	err := uc.AddGalleryReference(context.Background(), "mon_sac", image.NewRGBA(image.Rect(0, 0, 10, 10)))
	if !errors.Is(err, boom) {
		t.Fatalf("AddGalleryReference() error = %v, want %v", err, boom)
	}
}

// TestUseCase_AddGalleryReference_RespectsCancelledContext exercises the
// ctx.Done() early-exit added 2026-08-12 (use_case.go's doc comment on
// AddGalleryReference) — a pre-cancelled ctx must fail fast with
// ctx.Err() rather than paying for EncodeImage. Distinguished from
// PropagatesEncodeError above by using an encoder that would succeed if
// called at all, and asserting it's never called.
func TestUseCase_AddGalleryReference_RespectsCancelledContext(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := uc.AddGalleryReference(ctx, "mon_sac", image.NewRGBA(image.Rect(0, 0, 10, 10)))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AddGalleryReference() error = %v, want context.Canceled", err)
	}
	if _, ok := uc.gallery.Get("mon_sac"); ok {
		t.Fatal("gallery entry stored despite cancelled context")
	}
}

func TestUseCase_GalleryReference_RemoveRenameEnableList(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	crop := image.NewRGBA(image.Rect(0, 0, 10, 10))
	ctx := context.Background()

	if err := uc.AddGalleryReference(ctx, "thing", crop); err != nil {
		t.Fatalf("AddGalleryReference() error = %v", err)
	}

	if err := uc.RenameGalleryReference(ctx, "thing", "renamed"); err != nil {
		t.Fatalf("RenameGalleryReference() error = %v", err)
	}
	if err := uc.SetGalleryReferenceEnabled(ctx, "renamed", false); err != nil {
		t.Fatalf("SetGalleryReferenceEnabled() error = %v", err)
	}

	list := uc.ListGalleryReferences(ctx)
	if len(list) != 1 || list[0].Name != "renamed" || list[0].Enabled {
		t.Fatalf("ListGalleryReferences() = %+v, want [{renamed false}]", list)
	}

	uc.RemoveGalleryReference(ctx, "renamed")
	if got := uc.ListGalleryReferences(ctx); len(got) != 0 {
		t.Fatalf("ListGalleryReferences() after Remove = %+v, want empty", got)
	}
}

// TestUseCase_AddGalleryReference_SecondCallAppendsAnImage — 2026-08-13,
// multi-image entries: calling AddGalleryReference again with a name that
// already has a photo must add a second one, not error (the old
// behavior, still covered structurally by
// storage.GalleryStorage.AddImage's own doc comment/tests) — this test
// exercises it through the use case layer specifically, since that's
// the layer a REST caller (and therefore a GUI "add another photo to an
// existing term" flow) actually goes through.
func TestUseCase_AddGalleryReference_SecondCallAppendsAnImage(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	ctx := context.Background()
	crop := image.NewRGBA(image.Rect(0, 0, 10, 10))

	if err := uc.AddGalleryReference(ctx, "thing", crop); err != nil {
		t.Fatalf("first AddGalleryReference() error = %v", err)
	}
	if err := uc.AddGalleryReference(ctx, "thing", crop); err != nil {
		t.Fatalf("second AddGalleryReference() error = %v, want it to append, not reject", err)
	}

	list := uc.ListGalleryReferences(ctx)
	if len(list) != 1 || len(list[0].Images) != 2 {
		t.Fatalf("ListGalleryReferences() = %+v, want 1 entry with 2 images", list)
	}
}

// TestUseCase_RemoveGalleryImage_OneOfSeveral exercises the "v2" delete
// flow (alongside whole-entry RemoveGalleryReference, "v1") — removing
// one image by ID leaves the entry's other image(s) intact.
func TestUseCase_RemoveGalleryImage_OneOfSeveral(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	ctx := context.Background()
	crop := image.NewRGBA(image.Rect(0, 0, 10, 10))

	_ = uc.AddGalleryReference(ctx, "thing", crop)
	_ = uc.AddGalleryReference(ctx, "thing", crop)

	list := uc.ListGalleryReferences(ctx)
	if len(list) != 1 || len(list[0].Images) != 2 {
		t.Fatalf("ListGalleryReferences() = %+v, want 1 entry with 2 images", list)
	}

	uc.RemoveGalleryImage(ctx, "thing", list[0].Images[0].ID)

	list = uc.ListGalleryReferences(ctx)
	if len(list) != 1 || len(list[0].Images) != 1 {
		t.Fatalf("ListGalleryReferences() after RemoveGalleryImage = %+v, want 1 entry with 1 image", list)
	}
}

// TestUseCase_RemoveGalleryImage_LastOneDeletesTheWholeEntry — see
// storage.GalleryStorage.RemoveImage's own doc comment for the rationale
// (an entry with zero reference photos can never match anything).
func TestUseCase_RemoveGalleryImage_LastOneDeletesTheWholeEntry(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	ctx := context.Background()
	crop := image.NewRGBA(image.Rect(0, 0, 10, 10))

	_ = uc.AddGalleryReference(ctx, "thing", crop)
	list := uc.ListGalleryReferences(ctx)
	uc.RemoveGalleryImage(ctx, "thing", list[0].Images[0].ID)

	if got := uc.ListGalleryReferences(ctx); len(got) != 0 {
		t.Fatalf("ListGalleryReferences() after removing the only image = %+v, want empty", got)
	}
}

// TestUseCase_GetGalleryThumbnail_RoundTrips confirms AddGalleryReference
// actually stores a usable thumbnail (not just an embedding) — the whole
// point of keeping the crop around instead of discarding it after
// encoding (2026-08-13).
func TestUseCase_GetGalleryThumbnail_RoundTrips(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	ctx := context.Background()
	crop := image.NewRGBA(image.Rect(0, 0, 10, 10))

	_ = uc.AddGalleryReference(ctx, "thing", crop)
	list := uc.ListGalleryReferences(ctx)

	data, ok := uc.GetGalleryThumbnail(ctx, "thing", list[0].Images[0].ID)
	if !ok || len(data) == 0 {
		t.Fatalf("GetGalleryThumbnail() = %v, ok=%v, want non-empty JPEG bytes", data, ok)
	}
}

// TestUseCase_SetGalleryCocoClass — Bibliothèque model (2026-08-13,
// docs/gui/design-brief.md § Bibliothèque, screen 4c/4d): a Term's linked
// COCO class must be a real class name of the active model, or empty to
// clear an existing link — anything else is screen 4d's "warning" state,
// surfaced here as an error.
func TestUseCase_SetGalleryCocoClass(t *testing.T) {
	tests := []struct {
		name      string
		cocoClass string
		wantErr   bool
	}{
		{name: "valid COCO class", cocoClass: "person", wantErr: false},
		{name: "clears the link", cocoClass: "", wantErr: false},
		{name: "not a COCO class", cocoClass: "maillot", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoder := &mockSemanticEncoder{}
			u := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
			ctx := context.Background()
			_ = u.AddGalleryReference(ctx, "thing", image.NewRGBA(image.Rect(0, 0, 10, 10)))

			err := u.SetGalleryCocoClass(ctx, "thing", tt.cocoClass)
			if (err != nil) != tt.wantErr {
				t.Fatalf("SetGalleryCocoClass(%q) error = %v, wantErr %v", tt.cocoClass, err, tt.wantErr)
			}
			if !tt.wantErr {
				list := u.ListGalleryReferences(ctx)
				if len(list) != 1 || list[0].CocoClass != tt.cocoClass {
					t.Fatalf("ListGalleryReferences() = %+v, want CocoClass %q", list, tt.cocoClass)
				}
			}
		})
	}
}

// TestUseCase_Collections_CRUD exercises the Bibliothèque Collection
// lifecycle end to end through the use case layer: create, link an
// existing Term, list, unlink, rename, delete.
func TestUseCase_Collections_CRUD(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	u := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	ctx := context.Background()
	_ = u.AddGalleryReference(ctx, "thing", image.NewRGBA(image.Rect(0, 0, 10, 10)))

	if err := u.CreateCollection(ctx, "Manchester Team", []string{"football", "2026"}); err != nil {
		t.Fatalf("CreateCollection() error = %v", err)
	}
	if err := u.CreateCollection(ctx, "Manchester Team", nil); err == nil {
		t.Fatal("CreateCollection() with a name that already exists should error")
	}

	if err := u.AddTermToCollection(ctx, "Manchester Team", "thing"); err != nil {
		t.Fatalf("AddTermToCollection() error = %v", err)
	}

	list := u.ListCollections(ctx)
	if len(list) != 1 || list[0].Name != "Manchester Team" || len(list[0].Terms) != 1 || list[0].Terms[0] != "thing" {
		t.Fatalf("ListCollections() = %+v, want one Collection named %q with Term %q", list, "Manchester Team", "thing")
	}

	u.RemoveTermFromCollection(ctx, "Manchester Team", "thing")
	list = u.ListCollections(ctx)
	if len(list) != 1 || len(list[0].Terms) != 0 {
		t.Fatalf("ListCollections() after RemoveTermFromCollection = %+v, want zero Terms", list)
	}

	if err := u.RenameCollection(ctx, "Manchester Team", "Adversaires"); err != nil {
		t.Fatalf("RenameCollection() error = %v", err)
	}

	u.DeleteCollection(ctx, "Adversaires")
	if got := u.ListCollections(ctx); len(got) != 0 {
		t.Fatalf("ListCollections() after DeleteCollection = %+v, want empty", got)
	}
}

// TestUseCase_AddTermToCollection_RejectsUnknownTerm — a Collection
// referencing a name that isn't a real Term yet would silently produce a
// Bibliothèque screen showing a term with no photos, which the model says
// can't exist (screen 4a: "un Terme sans photo n'existe pas") — rejected
// up front instead, see AddTermToCollection's own doc comment.
func TestUseCase_AddTermToCollection_RejectsUnknownTerm(t *testing.T) {
	u := newTestUseCase(&mockObjectDetector{}, &mockSemanticEncoder{}, &mockAlertSender{})
	ctx := context.Background()
	_ = u.CreateCollection(ctx, "Manchester Team", nil)

	if err := u.AddTermToCollection(ctx, "Manchester Team", "does-not-exist"); err == nil {
		t.Fatal("AddTermToCollection() with an unknown Term should error")
	}
}
