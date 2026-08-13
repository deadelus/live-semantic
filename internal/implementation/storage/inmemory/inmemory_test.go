package inmemory

import "testing"

// COCO-collision rejection isn't tested here — that validation moved to
// application/uc.UseCase.AddGalleryReference/RenameGalleryReference
// during the 2026-08-12 port extraction (this package must not know what
// a COCO class is) — see internal/application/uc/uc_gallery_test.go.

var thumb = []byte{0xFF, 0xD8, 0xFF} // stand-in JPEG magic bytes, content irrelevant to this package

func TestGallery_AddImageThenGet(t *testing.T) {
	g := New()
	emb := []float32{1, 0, 0}
	if _, err := g.AddImage("mon_sac", emb, thumb); err != nil {
		t.Fatalf("AddImage() error = %v", err)
	}
	got, ok := g.Get("mon_sac")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if len(got) != 1 || len(got[0]) != len(emb) || got[0][0] != emb[0] {
		t.Fatalf("Get() = %v, want [%v]", got, emb)
	}
}

func TestGallery_AddImage_SecondCallAppendsNotRejects(t *testing.T) {
	g := New()
	if _, err := g.AddImage("mon_sac", []float32{1, 0}, thumb); err != nil {
		t.Fatalf("first AddImage() error = %v", err)
	}
	if _, err := g.AddImage("mon_sac", []float32{0, 1}, thumb); err != nil {
		t.Fatalf("second AddImage() error = %v, want it to append, not reject", err)
	}
	got, ok := g.Get("mon_sac")
	if !ok || len(got) != 2 {
		t.Fatalf("Get() = %v, ok=%v, want 2 embeddings", got, ok)
	}
}

func TestGallery_AddImage_ReturnsDistinctIDs(t *testing.T) {
	g := New()
	id1, _ := g.AddImage("thing", []float32{1}, thumb)
	id2, _ := g.AddImage("thing", []float32{0, 1}, thumb)
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("image IDs = %q, %q, want two distinct non-empty IDs", id1, id2)
	}
}

func TestGallery_Get_UnknownName(t *testing.T) {
	g := New()
	if _, ok := g.Get("nope"); ok {
		t.Fatal("Get() ok = true for a name never added, want false")
	}
}

func TestGallery_Get_NilGallery(t *testing.T) {
	var g *Gallery
	if _, ok := g.Get("anything"); ok {
		t.Fatal("Get() on a nil *Gallery should return ok=false, not panic")
	}
}

func TestGallery_AddImage_EmptyNameRejected(t *testing.T) {
	g := New()
	if _, err := g.AddImage("", []float32{1}, thumb); err == nil {
		t.Fatal("AddImage() with an empty name should error")
	}
}

func TestGallery_AddImage_EmptyEmbeddingRejected(t *testing.T) {
	g := New()
	if _, err := g.AddImage("thing", nil, thumb); err == nil {
		t.Fatal("AddImage() with an empty embedding should error")
	}
}

func TestGallery_AddImage_EmptyThumbnailRejected(t *testing.T) {
	g := New()
	if _, err := g.AddImage("thing", []float32{1}, nil); err == nil {
		t.Fatal("AddImage() with an empty thumbnail should error")
	}
}

func TestGallery_AddImage_CaseAndWhitespaceNormalized(t *testing.T) {
	g := New()
	if _, err := g.AddImage("  Mon Sac  ", []float32{1}, thumb); err != nil {
		t.Fatalf("AddImage() error = %v", err)
	}
	if _, ok := g.Get("mon sac"); !ok {
		t.Fatal("Get(\"mon sac\") ok = false, want true (name should be normalized like any other filter key)")
	}
}

func TestGallery_RemoveImage_LastOneDeletesTheWholeEntry(t *testing.T) {
	g := New()
	id, _ := g.AddImage("thing", []float32{1}, thumb)
	g.RemoveImage("thing", id)
	if _, ok := g.Get("thing"); ok {
		t.Fatal("entry still present after removing its only image")
	}
	if got := g.List(); len(got) != 0 {
		t.Fatalf("List() = %+v, want empty (entry should be gone, not left with zero images)", got)
	}
}

func TestGallery_RemoveImage_OneOfSeveralKeepsTheOthers(t *testing.T) {
	g := New()
	id1, _ := g.AddImage("thing", []float32{1}, thumb)
	_, _ = g.AddImage("thing", []float32{0, 1}, thumb)
	g.RemoveImage("thing", id1)

	got, ok := g.Get("thing")
	if !ok || len(got) != 1 {
		t.Fatalf("Get() = %v, ok=%v, want 1 remaining embedding", got, ok)
	}
}

func TestGallery_RemoveImage_IsIdempotent(t *testing.T) {
	g := New()
	id, _ := g.AddImage("thing", []float32{1}, thumb)
	g.RemoveImage("thing", id)
	g.RemoveImage("thing", id) // must not panic the second time
	g.RemoveImage("nope", "nope")
}

func TestGallery_Remove_IsIdempotent(t *testing.T) {
	g := New()
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	g.Remove("thing")
	g.Remove("thing") // must not panic/error the second time
	if _, ok := g.Get("thing"); ok {
		t.Fatal("entry still present after Remove()")
	}
}

func TestGallery_Remove_DeletesEveryImage(t *testing.T) {
	g := New()
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	_, _ = g.AddImage("thing", []float32{0, 1}, thumb)
	g.Remove("thing")
	if _, ok := g.Get("thing"); ok {
		t.Fatal("entry still present after Remove(), want every image gone")
	}
}

func TestGallery_SetEnabled_DisabledEntryNotReturnedByGet(t *testing.T) {
	g := New()
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	if err := g.SetEnabled("thing", false); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if _, ok := g.Get("thing"); ok {
		t.Fatal("Get() ok = true for a disabled entry, want false (must behave as absent to a filter lookup)")
	}
	if err := g.SetEnabled("thing", true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if _, ok := g.Get("thing"); !ok {
		t.Fatal("Get() ok = false after re-enabling, want true")
	}
}

func TestGallery_SetEnabled_UnknownNameErrors(t *testing.T) {
	g := New()
	if err := g.SetEnabled("nope", true); err == nil {
		t.Fatal("SetEnabled() on an unknown name should error")
	}
}

func TestGallery_Rename(t *testing.T) {
	g := New()
	_, _ = g.AddImage("old_name", []float32{1}, thumb)
	if err := g.Rename("old_name", "new_name"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if _, ok := g.Get("old_name"); ok {
		t.Fatal("old name still resolves after Rename()")
	}
	if _, ok := g.Get("new_name"); !ok {
		t.Fatal("new name doesn't resolve after Rename()")
	}
}

func TestGallery_Rename_TargetCollisionRejected(t *testing.T) {
	g := New()
	_, _ = g.AddImage("a", []float32{1}, thumb)
	_, _ = g.AddImage("b", []float32{0, 1}, thumb)
	if err := g.Rename("a", "b"); err == nil {
		t.Fatal("Rename() onto an existing name should error, not silently overwrite")
	}
}

func TestGallery_Rename_MissingSourceErrors(t *testing.T) {
	g := New()
	if err := g.Rename("nope", "new_name"); err == nil {
		t.Fatal("Rename() on a missing source should error")
	}
}

func TestGallery_Thumbnail_RoundTrips(t *testing.T) {
	g := New()
	id, _ := g.AddImage("thing", []float32{1}, thumb)
	got, ok := g.Thumbnail("thing", id)
	if !ok || string(got) != string(thumb) {
		t.Fatalf("Thumbnail() = %v, ok=%v, want %v, true", got, ok, thumb)
	}
}

func TestGallery_Thumbnail_UnknownIDErrors(t *testing.T) {
	g := New()
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	if _, ok := g.Thumbnail("thing", "nope"); ok {
		t.Fatal("Thumbnail() ok = true for an unknown image ID, want false")
	}
}

func TestGallery_List_SortedByName_IncludesDisabled(t *testing.T) {
	g := New()
	_, _ = g.AddImage("zebra_bag", []float32{1}, thumb)
	_, _ = g.AddImage("apple_box", []float32{1}, thumb)
	_ = g.SetEnabled("zebra_bag", false)

	got := g.List()
	if len(got) != 2 {
		t.Fatalf("List() returned %d entries, want 2 (including the disabled one)", len(got))
	}
	if got[0].Name != "apple_box" || got[1].Name != "zebra_bag" {
		t.Fatalf("List() = %+v, want sorted by name", got)
	}
	if got[1].Enabled {
		t.Fatalf("List()[1].Enabled = true, want false (zebra_bag was disabled)")
	}
	if len(got[0].Images) != 1 {
		t.Fatalf("List()[0].Images = %+v, want 1 image", got[0].Images)
	}
}

func TestGallery_List_MultipleImagesPerEntry(t *testing.T) {
	g := New()
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	_, _ = g.AddImage("thing", []float32{0, 1}, thumb)

	got := g.List()
	if len(got) != 1 || len(got[0].Images) != 2 {
		t.Fatalf("List() = %+v, want 1 entry with 2 images", got)
	}
}
