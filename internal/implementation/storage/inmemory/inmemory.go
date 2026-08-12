// Package inmemory is the only storage.GalleryStorage adapter
// today — a thread-safe in-memory map. No vectorial database at this
// scale — a handful to a few dozen entries, linear scan on List/Get is
// plenty. Process-wide, not per-session (multi-flux: a *Gallery is shared
// across every application/uc.UseCase a session.Manager creates — see
// session.go's doc comment — reusing named references across sessions is
// the whole point of naming them).
package inmemory

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/storage"
)

// Gallery implements storage.GalleryStorage.
type Gallery struct {
	mu      sync.RWMutex
	entries map[string]*entry
}

type entry struct {
	name      string
	embedding entities.Embedding
	enabled   bool
}

var _ storage.GalleryStorage = (*Gallery)(nil)

// New creates an empty Gallery.
func New() *Gallery {
	return &Gallery{entries: make(map[string]*entry)}
}

// normalize canonicalizes a storage key (case/whitespace) — a plain
// duplicate of application/uc.normalizeFilter's one-liner rather than an
// import of it: this package must not depend on application/uc (that
// would invert the dependency direction implementation/ -> application/
// this project never allows), and the logic is trivial enough that
// duplicating it here is cheaper than the alternative (promoting it to
// some new shared low-level package for a two-line function used by
// exactly two callers).
func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Add — see storage.GalleryStorage.
func (g *Gallery) Add(name string, embedding entities.Embedding) error {
	name = normalize(name)
	if name == "" {
		return fmt.Errorf("gallery entry name cannot be empty")
	}
	if len(embedding) == 0 {
		return fmt.Errorf("gallery entry %q: embedding cannot be empty", name)
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.entries[name]; exists {
		return fmt.Errorf("gallery entry %q already exists", name)
	}
	g.entries[name] = &entry{name: name, embedding: embedding, enabled: true}
	return nil
}

// Remove — see storage.GalleryStorage.
func (g *Gallery) Remove(name string) {
	name = normalize(name)
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.entries, name)
}

// Rename — see storage.GalleryStorage.
func (g *Gallery) Rename(oldName, newName string) error {
	oldName = normalize(oldName)
	newName = normalize(newName)
	if newName == "" {
		return fmt.Errorf("new gallery entry name cannot be empty")
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.entries[oldName]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", oldName)
	}
	if _, exists := g.entries[newName]; exists {
		return fmt.Errorf("gallery entry %q already exists", newName)
	}
	delete(g.entries, oldName)
	e.name = newName
	g.entries[newName] = e
	return nil
}

// SetEnabled — see storage.GalleryStorage.
func (g *Gallery) SetEnabled(name string, enabled bool) error {
	name = normalize(name)
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.entries[name]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	e.enabled = enabled
	return nil
}

// Get — see storage.GalleryStorage. Nil-receiver-safe (a nil *Gallery
// behaves as empty) — application/uc.NewUseCase's gallery parameter used
// to allow a nil *ReferenceGallery pre-refactor; kept for callers that
// might still hold a nil Repository value structurally, cheap to
// preserve.
func (g *Gallery) Get(name string) (entities.Embedding, bool) {
	if g == nil {
		return nil, false
	}
	name = normalize(name)
	g.mu.RLock()
	defer g.mu.RUnlock()
	e, ok := g.entries[name]
	if !ok || !e.enabled {
		return nil, false
	}
	return e.embedding, true
}

// List — see storage.GalleryStorage.
func (g *Gallery) List() []storage.Gallery {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]storage.Gallery, 0, len(g.entries))
	for _, e := range g.entries {
		out = append(out, storage.Gallery{Name: e.name, Embedding: e.embedding, Enabled: e.enabled})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
