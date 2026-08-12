package uc

import (
	"fmt"
	"sort"
	"sync"

	"live-semantic/internal/domain/entities"
)

// ReferenceGallery is the in-memory {name, embedding} store for "recognize
// by reference image" filter terms — TODO.md § D "reconnaissance par
// référence image" / § H1 "Galerie de références", promoted 2026-08-12
// (docs/adr/clip-backend.md § 24) into a third term family alongside COCO
// labels and free-text CLIP terms: a gallery entry's name becomes usable
// directly as a filter key (newTrackManager checks the gallery before
// falling back to EncodeText — see its doc comment), matched via
// image↔image cosine similarity instead of text↔image. No vectorial
// database at this scale (TODO.md § D's own note) — a handful to a few
// dozen entries, linear scan is plenty.
//
// Thread-safe (RWMutex): read from newTrackManager (parsing a filter,
// potentially concurrent with a REST CRUD call touching the gallery).
// Process-wide, not per-session — H1 minimal scope has one recognition
// session anyway (TODO.md § H1 "Multi-flux" not done yet), and reusing
// named references across sessions is the whole point of naming them.
type ReferenceGallery struct {
	mu      sync.RWMutex
	entries map[string]*galleryEntry
}

type galleryEntry struct {
	Name      string
	Embedding entities.Embedding
	// Enabled lets an entry be kept (not deleted, so its embedding/name
	// isn't lost) but excluded from matching — "activable/désactivable
	// individuellement" per docs/gui/spec.md § 3.2.
	Enabled bool
}

// GalleryEntryInfo is the read-only, embedding-free view of a gallery
// entry — what List returns. Embedding vectors (512 floats for CLIP
// ViT-B/32) have no use client-side and shouldn't cross the REST
// boundary.
type GalleryEntryInfo struct {
	Name    string
	Enabled bool
}

// NewReferenceGallery creates an empty gallery.
func NewReferenceGallery() *ReferenceGallery {
	return &ReferenceGallery{entries: make(map[string]*galleryEntry)}
}

// Add registers a new entry, enabled by default. Rejects an empty name or
// a name that collides with an existing gallery entry OR a COCO class
// (isCOCOLabel) — a gallery entry sharing a COCO name would silently
// never be reachable as a filter term (newTrackManager checks COCO
// labels first, unconditionally, before ever consulting the gallery —
// see its doc comment), which would look like the gallery entry doesn't
// work rather than explain why.
func (g *ReferenceGallery) Add(name string, embedding entities.Embedding) error {
	name = normalizeFilter(name)
	if name == "" {
		return fmt.Errorf("gallery entry name cannot be empty")
	}
	if isCOCOLabel(name) {
		return fmt.Errorf("gallery entry name %q collides with a COCO class — it would never be reachable as a filter term (COCO labels are checked first)", name)
	}
	if len(embedding) == 0 {
		return fmt.Errorf("gallery entry %q: embedding cannot be empty", name)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.entries[name]; exists {
		return fmt.Errorf("gallery entry %q already exists", name)
	}
	g.entries[name] = &galleryEntry{Name: name, Embedding: embedding, Enabled: true}
	return nil
}

// Remove deletes an entry — a no-op (not an error) if it doesn't exist,
// same idempotent-delete convention as most REST DELETE semantics.
func (g *ReferenceGallery) Remove(name string) {
	name = normalizeFilter(name)
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, name)
}

// Rename changes an entry's key. Rejects a missing source, an empty
// target, or a target that already exists (ambiguous which entry should
// win) — same fail-fast philosophy as everything else in this package
// rather than silently overwriting.
func (g *ReferenceGallery) Rename(oldName, newName string) error {
	oldName = normalizeFilter(oldName)
	newName = normalizeFilter(newName)
	if newName == "" {
		return fmt.Errorf("new gallery entry name cannot be empty")
	}
	if isCOCOLabel(newName) {
		return fmt.Errorf("gallery entry name %q collides with a COCO class — it would never be reachable as a filter term", newName)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.entries[oldName]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", oldName)
	}
	if _, exists := g.entries[newName]; exists {
		return fmt.Errorf("gallery entry %q already exists", newName)
	}
	delete(g.entries, oldName)
	entry.Name = newName
	g.entries[newName] = entry
	return nil
}

// SetEnabled toggles an entry without deleting it. Errors on a missing
// entry rather than silently doing nothing.
func (g *ReferenceGallery) SetEnabled(name string, enabled bool) error {
	name = normalizeFilter(name)
	g.mu.Lock()
	defer g.mu.Unlock()
	entry, ok := g.entries[name]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	entry.Enabled = enabled
	return nil
}

// Get returns name's embedding — ok is false if the entry doesn't exist
// OR is disabled (a disabled entry must behave as if absent to a filter
// term looking it up, not as a silent empty match). Consulted by
// newTrackManager per semantic (non-COCO) filter term.
func (g *ReferenceGallery) Get(name string) (entities.Embedding, bool) {
	if g == nil {
		return nil, false
	}
	name = normalizeFilter(name)
	g.mu.RLock()
	defer g.mu.RUnlock()
	entry, ok := g.entries[name]
	if !ok || !entry.Enabled {
		return nil, false
	}
	return entry.Embedding, true
}

// List returns every entry (enabled or not) sorted by name, for a REST
// GET — embedding-free (GalleryEntryInfo), see its doc comment.
func (g *ReferenceGallery) List() []GalleryEntryInfo {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]GalleryEntryInfo, 0, len(g.entries))
	for _, e := range g.entries {
		out = append(out, GalleryEntryInfo{Name: e.Name, Enabled: e.Enabled})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
