package diskgallery

import (
	"path/filepath"
	"testing"
)

var thumb = []byte{0xFF, 0xD8, 0xFF}

func TestNew_CreatesRootDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "gallery")
	if _, err := New(root); err != nil {
		t.Fatalf("New() error = %v", err)
	}
}

func TestAddImageThenGet(t *testing.T) {
	g, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := g.AddImage("mon_sac", []float32{1, 0, 0}, thumb); err != nil {
		t.Fatalf("AddImage() error = %v", err)
	}
	got, ok := g.Get("mon_sac")
	if !ok || len(got) != 1 {
		t.Fatalf("Get() = %v, ok=%v, want 1 embedding", got, ok)
	}
}

func TestAddImage_AppendsUnderSameName(t *testing.T) {
	g, _ := New(t.TempDir())
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	_, _ = g.AddImage("thing", []float32{0, 1}, thumb)

	got, ok := g.Get("thing")
	if !ok || len(got) != 2 {
		t.Fatalf("Get() = %v, ok=%v, want 2 embeddings", got, ok)
	}
}

func TestThumbnail_RoundTrips(t *testing.T) {
	g, _ := New(t.TempDir())
	id, _ := g.AddImage("thing", []float32{1}, thumb)

	got, ok := g.Thumbnail("thing", id)
	if !ok || string(got) != string(thumb) {
		t.Fatalf("Thumbnail() = %v, ok=%v, want %v true", got, ok, thumb)
	}
}

// TestPersistsAcrossRestart is the whole point of this package — a fresh
// *Gallery pointed at the same root, simulating a process restart, must
// see everything the previous instance wrote (entries, all embeddings,
// enabled state, and thumbnail bytes), and must not reuse an image ID
// already on disk.
func TestPersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()

	g1, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	id1, _ := g1.AddImage("mon_sac", []float32{1, 0}, thumb)
	_, _ = g1.AddImage("mon_sac", []float32{0, 1}, thumb)
	_ = g1.SetEnabled("mon_sac", false)

	g2, err := New(root)
	if err != nil {
		t.Fatalf("second New() error = %v", err)
	}

	// Disabled entries behave as absent to Get() — verified via List()
	// instead, same convention as inmemory's own tests.
	list := g2.List()
	if len(list) != 1 || list[0].Name != "mon_sac" || list[0].Enabled {
		t.Fatalf("List() after restart = %+v, want 1 disabled entry", list)
	}
	if len(list[0].Images) != 2 {
		t.Fatalf("List()[0].Images after restart = %+v, want 2", list[0].Images)
	}

	got, ok := g2.Thumbnail("mon_sac", id1)
	if !ok || string(got) != string(thumb) {
		t.Fatalf("Thumbnail() after restart = %v, ok=%v, want the original bytes", got, ok)
	}

	// A new image added post-restart must not collide with img IDs
	// already used pre-restart.
	id3, _ := g2.AddImage("mon_sac", []float32{1, 1}, thumb)
	if id3 == id1 {
		t.Fatalf("new image ID %q collides with a pre-restart ID", id3)
	}
}

func TestRemoveImage_LastOneDeletesTheWholeEntryOnDisk(t *testing.T) {
	root := t.TempDir()
	g, _ := New(root)
	id, _ := g.AddImage("thing", []float32{1}, thumb)
	g.RemoveImage("thing", id)

	if _, ok := g.Get("thing"); ok {
		t.Fatal("entry still present after removing its only image")
	}

	// Confirmed on disk too, not just in memory — a restart must agree.
	g2, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := g2.List(); len(got) != 0 {
		t.Fatalf("List() after restart = %+v, want empty", got)
	}
}

func TestRemoveImage_OneOfSeveralKeepsTheOthers(t *testing.T) {
	g, _ := New(t.TempDir())
	id1, _ := g.AddImage("thing", []float32{1}, thumb)
	_, _ = g.AddImage("thing", []float32{0, 1}, thumb)
	g.RemoveImage("thing", id1)

	got, ok := g.Get("thing")
	if !ok || len(got) != 1 {
		t.Fatalf("Get() = %v, ok=%v, want 1 remaining embedding", got, ok)
	}
	if _, ok := g.Thumbnail("thing", id1); ok {
		t.Fatal("Thumbnail() still returns the removed image, want it gone")
	}
}

func TestRemove_DeletesDirectoryOnDisk(t *testing.T) {
	root := t.TempDir()
	g, _ := New(root)
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	g.Remove("thing")

	g2, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := g2.List(); len(got) != 0 {
		t.Fatalf("List() after restart = %+v, want empty", got)
	}
}

func TestRename_MovesEntryDirAndSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	g, _ := New(root)
	id, _ := g.AddImage("old_name", []float32{1}, thumb)
	if err := g.Rename("old_name", "new_name"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	g2, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := g2.Get("old_name"); ok {
		t.Fatal("old name still resolves after Rename() + restart")
	}
	if _, ok := g2.Get("new_name"); !ok {
		t.Fatal("new name doesn't resolve after Rename() + restart")
	}
	if _, ok := g2.Thumbnail("new_name", id); !ok {
		t.Fatal("thumbnail didn't move with the renamed entry")
	}
}

func TestRename_TargetCollisionRejected(t *testing.T) {
	g, _ := New(t.TempDir())
	_, _ = g.AddImage("a", []float32{1}, thumb)
	_, _ = g.AddImage("b", []float32{0, 1}, thumb)
	if err := g.Rename("a", "b"); err == nil {
		t.Fatal("Rename() onto an existing name should error, not silently overwrite")
	}
}

func TestSetEnabled_PersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	g, _ := New(root)
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	if err := g.SetEnabled("thing", false); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	g2, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	list := g2.List()
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("List() after restart = %+v, want 1 disabled entry", list)
	}
}

// TestSetCocoClass_PersistsAcrossRestart — Bibliothèque model (2026-08-13,
// docs/gui/design-brief.md § Bibliothèque, screen 4c) — a Term's linked
// COCO class must survive a restart just like every other field.
func TestSetCocoClass_PersistsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	g, _ := New(root)
	_, _ = g.AddImage("thing", []float32{1}, thumb)
	if err := g.SetCocoClass("thing", "person"); err != nil {
		t.Fatalf("SetCocoClass() error = %v", err)
	}

	g2, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	list := g2.List()
	if len(list) != 1 || list[0].CocoClass != "person" {
		t.Fatalf("List() after restart = %+v, want CocoClass %q", list, "person")
	}

	if err := g2.SetCocoClass("thing", ""); err != nil {
		t.Fatalf("SetCocoClass() (clear) error = %v", err)
	}
	if list := g2.List(); len(list) != 1 || list[0].CocoClass != "" {
		t.Fatalf("List() after clearing = %+v, want empty CocoClass", list)
	}
}

func TestSetCocoClass_UnknownEntryErrors(t *testing.T) {
	g, _ := New(t.TempDir())
	if err := g.SetCocoClass("does-not-exist", "person"); err == nil {
		t.Fatal("SetCocoClass() on an unknown entry should error")
	}
}

func TestSlug_NameWithSpecialCharactersIsFilesystemSafe(t *testing.T) {
	g, _ := New(t.TempDir())
	if _, err := g.AddImage("un sac / très cool!", []float32{1}, thumb); err != nil {
		t.Fatalf("AddImage() with unusual characters in the name error = %v", err)
	}
	if _, ok := g.Get("un sac / très cool!"); !ok {
		t.Fatal("Get() with the original name doesn't resolve")
	}
}

func TestAddImage_EmptyThumbnailRejected(t *testing.T) {
	g, _ := New(t.TempDir())
	if _, err := g.AddImage("thing", []float32{1}, nil); err == nil {
		t.Fatal("AddImage() with an empty thumbnail should error")
	}
}
