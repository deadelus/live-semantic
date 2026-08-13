package diskgallery

import (
	"testing"
)

func TestCollections_New_CreatesRootDir(t *testing.T) {
	root := t.TempDir()
	if _, err := NewCollections(root); err != nil {
		t.Fatalf("NewCollections() error = %v", err)
	}
}

func TestCollections_CreateThenGet(t *testing.T) {
	c, err := NewCollections(t.TempDir())
	if err != nil {
		t.Fatalf("NewCollections() error = %v", err)
	}
	if err := c.Create("Manchester Team", []string{"football", "2026"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, ok := c.Get("Manchester Team")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Name != "manchester team" || len(got.Tags) != 2 || len(got.Terms) != 0 {
		t.Fatalf("Get() = %+v, want normalized name with 2 tags, 0 terms", got)
	}
}

func TestCollections_Create_DuplicateNameErrors(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	if err := c.Create("team", nil); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if err := c.Create("Team", nil); err == nil {
		t.Fatal("Create() with an already-used (case-insensitive) name should error")
	}
}

func TestCollections_Create_EmptyNameErrors(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	if err := c.Create("", nil); err == nil {
		t.Fatal("Create() with an empty name should error")
	}
}

func TestCollections_AddTermThenRemoveTerm(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	_ = c.Create("team", nil)

	if err := c.AddTerm("team", "maillot numéro 10"); err != nil {
		t.Fatalf("AddTerm() error = %v", err)
	}
	if err := c.AddTerm("team", "maillot numéro 10"); err != nil {
		t.Fatalf("AddTerm() (idempotent re-add) error = %v", err)
	}

	got, _ := c.Get("team")
	if len(got.Terms) != 1 {
		t.Fatalf("Get() = %+v, want exactly 1 Term despite adding twice", got)
	}

	c.RemoveTerm("team", "maillot numéro 10")
	got, _ = c.Get("team")
	if len(got.Terms) != 0 {
		t.Fatalf("Get() after RemoveTerm = %+v, want 0 Terms", got)
	}

	// Idempotent — removing again (or from an unknown collection) must not
	// panic/error.
	c.RemoveTerm("team", "maillot numéro 10")
	c.RemoveTerm("does-not-exist", "whatever")
}

func TestCollections_AddTerm_UnknownCollectionErrors(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	if err := c.AddTerm("does-not-exist", "thing"); err == nil {
		t.Fatal("AddTerm() on an unknown collection should error")
	}
}

func TestCollections_Rename(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	_ = c.Create("old", []string{"tag"})
	_ = c.AddTerm("old", "thing")

	if err := c.Rename("old", "new"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if _, ok := c.Get("old"); ok {
		t.Fatal("Get(old) still found after Rename()")
	}
	got, ok := c.Get("new")
	if !ok || len(got.Tags) != 1 || len(got.Terms) != 1 {
		t.Fatalf("Get(new) = %+v, ok=%v, want tags/terms carried over", got, ok)
	}
}

func TestCollections_Rename_TargetExistsErrors(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	_ = c.Create("a", nil)
	_ = c.Create("b", nil)
	if err := c.Rename("a", "b"); err == nil {
		t.Fatal("Rename() onto an existing name should error")
	}
}

func TestCollections_SetTags(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	_ = c.Create("team", []string{"old-tag"})
	if err := c.SetTags("team", []string{"football", "football", " 2026 "}); err != nil {
		t.Fatalf("SetTags() error = %v", err)
	}
	got, _ := c.Get("team")
	if len(got.Tags) != 2 {
		t.Fatalf("Get() = %+v, want tags deduplicated/trimmed to 2", got)
	}
}

func TestCollections_Delete(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	_ = c.Create("team", nil)
	c.Delete("team")
	if _, ok := c.Get("team"); ok {
		t.Fatal("Get() still found after Delete()")
	}
	// Idempotent.
	c.Delete("team")
}

func TestCollections_List_SortedByName(t *testing.T) {
	c, _ := NewCollections(t.TempDir())
	_ = c.Create("zebra", nil)
	_ = c.Create("alpha", nil)

	got := c.List()
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zebra" {
		t.Fatalf("List() = %+v, want [alpha zebra]", got)
	}
}

// TestCollections_PersistAcrossRestart is the whole point of this
// adapter, same rationale as diskgallery_test.go's own
// TestPersistsAcrossRestart — a fresh *Collections pointed at the same
// root must see everything a previous instance wrote.
func TestCollections_PersistAcrossRestart(t *testing.T) {
	root := t.TempDir()

	c1, err := NewCollections(root)
	if err != nil {
		t.Fatalf("NewCollections() error = %v", err)
	}
	if err := c1.Create("Manchester Team", []string{"football"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := c1.AddTerm("Manchester Team", "maillot 10"); err != nil {
		t.Fatalf("AddTerm() error = %v", err)
	}

	c2, err := NewCollections(root)
	if err != nil {
		t.Fatalf("second NewCollections() error = %v", err)
	}
	got, ok := c2.Get("Manchester Team")
	if !ok {
		t.Fatal("Get() on restarted instance ok = false, want true")
	}
	if len(got.Tags) != 1 || len(got.Terms) != 1 || got.Terms[0] != "maillot 10" {
		t.Fatalf("Get() after restart = %+v, want tags/terms surviving restart", got)
	}
}

// TestCollections_DeleteNeverTouchesGallery is a documentation-as-test
// check that Collections and Gallery are genuinely independent stores
// under the same root — deleting a Collection must not remove the
// Gallery entry (Term) it referenced (docs/gui/design-brief.md §
// Bibliothèque: "Retirer un Terme ne le supprime pas de la
// bibliothèque", the same independence holds in the other direction).
func TestCollections_DeleteNeverTouchesGallery(t *testing.T) {
	root := t.TempDir()
	g, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	c, err := NewCollections(root)
	if err != nil {
		t.Fatalf("NewCollections() error = %v", err)
	}

	if _, err := g.AddImage("thing", []float32{1}, thumb); err != nil {
		t.Fatalf("AddImage() error = %v", err)
	}
	_ = c.Create("team", nil)
	_ = c.AddTerm("team", "thing")

	c.Delete("team")

	if _, ok := g.Get("thing"); !ok {
		t.Fatal("Gallery entry disappeared after deleting a Collection that referenced it")
	}
}
