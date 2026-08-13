package inmemory

import "testing"

func TestGallery_SetCocoClass(t *testing.T) {
	g := New()
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	if err := g.SetCocoClass("thing", "person"); err != nil {
		t.Fatalf("SetCocoClass() error = %v", err)
	}
	list := g.List()
	if len(list) != 1 || list[0].CocoClass != "person" {
		t.Fatalf("List() = %+v, want CocoClass %q", list, "person")
	}

	if err := g.SetCocoClass("thing", ""); err != nil {
		t.Fatalf("SetCocoClass() (clear) error = %v", err)
	}
	if list := g.List(); len(list) != 1 || list[0].CocoClass != "" {
		t.Fatalf("List() after clearing = %+v, want empty CocoClass", list)
	}
}

func TestGallery_SetCocoClass_UnknownNameErrors(t *testing.T) {
	g := New()
	if err := g.SetCocoClass("nope", "person"); err == nil {
		t.Fatal("SetCocoClass() on an unknown name should error")
	}
}

func TestCollections_CreateThenGet(t *testing.T) {
	c := NewCollections()
	if err := c.Create("Manchester Team", []string{"football", "football", " 2026 "}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, ok := c.Get("Manchester Team")
	if !ok || got.Name != "manchester team" || len(got.Tags) != 2 || len(got.Terms) != 0 {
		t.Fatalf("Get() = %+v, ok=%v, want normalized name, deduped tags, 0 terms", got, ok)
	}
}

func TestCollections_Create_DuplicateNameErrors(t *testing.T) {
	c := NewCollections()
	_ = c.Create("team", nil)
	if err := c.Create("Team", nil); err == nil {
		t.Fatal("Create() with an already-used (case-insensitive) name should error")
	}
}

func TestCollections_AddTerm_IdempotentAndUnknownCollectionErrors(t *testing.T) {
	c := NewCollections()
	if err := c.AddTerm("nope", "thing"); err == nil {
		t.Fatal("AddTerm() on an unknown collection should error")
	}

	_ = c.Create("team", nil)
	_ = c.AddTerm("team", "thing")
	_ = c.AddTerm("team", "thing")
	got, _ := c.Get("team")
	if len(got.Terms) != 1 {
		t.Fatalf("Get() = %+v, want exactly 1 Term despite adding twice", got)
	}

	c.RemoveTerm("team", "thing")
	got, _ = c.Get("team")
	if len(got.Terms) != 0 {
		t.Fatalf("Get() after RemoveTerm = %+v, want 0 Terms", got)
	}
}

func TestCollections_RenameDeleteList(t *testing.T) {
	c := NewCollections()
	_ = c.Create("zebra", nil)
	_ = c.Create("alpha", nil)

	if err := c.Rename("zebra", "beta"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	got := c.List()
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("List() = %+v, want [alpha beta]", got)
	}

	c.Delete("alpha")
	if got := c.List(); len(got) != 1 || got[0].Name != "beta" {
		t.Fatalf("List() after Delete = %+v, want [beta]", got)
	}
}
