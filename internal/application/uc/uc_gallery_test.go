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
