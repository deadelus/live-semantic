package uc

import (
	"errors"
	"image"
	"testing"
)

func TestUseCase_AddGalleryReference_EncodesAndStores(t *testing.T) {
	encoder := &mockSemanticEncoder{scoreByCropSize: map[string]float32{"20x20": 0.42}}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})

	crop := image.NewRGBA(image.Rect(0, 0, 20, 20))
	if err := uc.AddGalleryReference("mon_sac", crop); err != nil {
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

	err := uc.AddGalleryReference("mon_sac", image.NewRGBA(image.Rect(0, 0, 10, 10)))
	if !errors.Is(err, boom) {
		t.Fatalf("AddGalleryReference() error = %v, want %v", err, boom)
	}
}

func TestUseCase_GalleryReference_RemoveRenameEnableList(t *testing.T) {
	encoder := &mockSemanticEncoder{}
	uc := newTestUseCase(&mockObjectDetector{}, encoder, &mockAlertSender{})
	crop := image.NewRGBA(image.Rect(0, 0, 10, 10))

	if err := uc.AddGalleryReference("thing", crop); err != nil {
		t.Fatalf("AddGalleryReference() error = %v", err)
	}

	if err := uc.RenameGalleryReference("thing", "renamed"); err != nil {
		t.Fatalf("RenameGalleryReference() error = %v", err)
	}
	if err := uc.SetGalleryReferenceEnabled("renamed", false); err != nil {
		t.Fatalf("SetGalleryReferenceEnabled() error = %v", err)
	}

	list := uc.ListGalleryReferences()
	if len(list) != 1 || list[0].Name != "renamed" || list[0].Enabled {
		t.Fatalf("ListGalleryReferences() = %+v, want [{renamed false}]", list)
	}

	uc.RemoveGalleryReference("renamed")
	if got := uc.ListGalleryReferences(); len(got) != 0 {
		t.Fatalf("ListGalleryReferences() after Remove = %+v, want empty", got)
	}
}
