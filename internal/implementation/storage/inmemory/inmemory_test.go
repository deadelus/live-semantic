package inmemory

import "testing"

// COCO-collision rejection isn't tested here — that validation moved to
// application/uc.UseCase.AddGalleryReference/RenameGalleryReference
// during the 2026-08-12 port extraction (this package must not know what
// a COCO class is) — see internal/application/uc/uc_gallery_test.go.

func TestGallery_AddThenGet(t *testing.T) {
	g := New()
	emb := []float32{1, 0, 0}
	if err := g.Add("mon_sac", emb); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	got, ok := g.Get("mon_sac")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if len(got) != len(emb) || got[0] != emb[0] {
		t.Fatalf("Get() = %v, want %v", got, emb)
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

func TestGallery_Add_EmptyNameRejected(t *testing.T) {
	g := New()
	if err := g.Add("", []float32{1}); err == nil {
		t.Fatal("Add() with an empty name should error")
	}
}

func TestGallery_Add_EmptyEmbeddingRejected(t *testing.T) {
	g := New()
	if err := g.Add("thing", nil); err == nil {
		t.Fatal("Add() with an empty embedding should error")
	}
}

func TestGallery_Add_DuplicateRejected(t *testing.T) {
	g := New()
	if err := g.Add("thing", []float32{1}); err != nil {
		t.Fatalf("first Add() error = %v", err)
	}
	if err := g.Add("thing", []float32{0, 1}); err == nil {
		t.Fatal("second Add() with the same name should error")
	}
}

func TestGallery_Add_CaseAndWhitespaceNormalized(t *testing.T) {
	g := New()
	if err := g.Add("  Mon Sac  ", []float32{1}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if _, ok := g.Get("mon sac"); !ok {
		t.Fatal("Get(\"mon sac\") ok = false, want true (name should be normalized like any other filter key)")
	}
}

func TestGallery_Remove_IsIdempotent(t *testing.T) {
	g := New()
	_ = g.Add("thing", []float32{1})
	g.Remove("thing")
	g.Remove("thing") // must not panic/error the second time
	if _, ok := g.Get("thing"); ok {
		t.Fatal("entry still present after Remove()")
	}
}

func TestGallery_SetEnabled_DisabledEntryNotReturnedByGet(t *testing.T) {
	g := New()
	_ = g.Add("thing", []float32{1})
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
	_ = g.Add("old_name", []float32{1})
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
	_ = g.Add("a", []float32{1})
	_ = g.Add("b", []float32{0, 1})
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

func TestGallery_List_SortedByName_IncludesDisabled(t *testing.T) {
	g := New()
	_ = g.Add("zebra_bag", []float32{1})
	_ = g.Add("apple_box", []float32{1})
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
}
