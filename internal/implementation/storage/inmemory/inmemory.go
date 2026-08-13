// Package inmemory is a thread-safe, process-RAM-only
// storage.GalleryStorage adapter — no disk, nothing survives a restart.
// Kept alongside implementation/storage/diskgallery (the persisted
// adapter main.go actually wires in) as the lightest possible
// implementation for tests/tools that don't care about persistence. No
// vectorial database at this scale — a handful to a few dozen entries,
// linear scan on List/Get is plenty.
package inmemory

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/storage"
)

// Gallery implements storage.GalleryStorage.
type Gallery struct {
	mu         sync.RWMutex
	entries    map[string]*entry
	nextImgSeq uint64
}

type entry struct {
	name    string
	enabled bool
	images  []image
}

type image struct {
	id        string
	embedding entities.Embedding
	thumbnail []byte
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
// some new shared low-level package for a two-line function used by a
// handful of callers).
func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// AddImage — see storage.GalleryStorage.
func (g *Gallery) AddImage(name string, embedding entities.Embedding, thumbnail []byte) (string, error) {
	name = normalize(name)
	if name == "" {
		return "", fmt.Errorf("gallery entry name cannot be empty")
	}
	if len(embedding) == 0 {
		return "", fmt.Errorf("gallery entry %q: embedding cannot be empty", name)
	}
	if len(thumbnail) == 0 {
		return "", fmt.Errorf("gallery entry %q: thumbnail cannot be empty", name)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	g.nextImgSeq++
	id := "img-" + strconv.FormatUint(g.nextImgSeq, 10)
	img := image{id: id, embedding: embedding, thumbnail: thumbnail}

	e, ok := g.entries[name]
	if !ok {
		e = &entry{name: name, enabled: true}
		g.entries[name] = e
	}
	e.images = append(e.images, img)
	return id, nil
}

// RemoveImage — see storage.GalleryStorage.
func (g *Gallery) RemoveImage(name, imageID string) {
	name = normalize(name)
	g.mu.Lock()
	defer g.mu.Unlock()

	e, ok := g.entries[name]
	if !ok {
		return
	}
	for i, img := range e.images {
		if img.id == imageID {
			e.images = append(e.images[:i], e.images[i+1:]...)
			break
		}
	}
	if len(e.images) == 0 {
		delete(g.entries, name)
	}
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
func (g *Gallery) Get(name string) ([]entities.Embedding, bool) {
	if g == nil {
		return nil, false
	}
	name = normalize(name)
	g.mu.RLock()
	defer g.mu.RUnlock()
	e, ok := g.entries[name]
	if !ok || !e.enabled || len(e.images) == 0 {
		return nil, false
	}
	out := make([]entities.Embedding, len(e.images))
	for i, img := range e.images {
		out[i] = img.embedding
	}
	return out, true
}

// Thumbnail — see storage.GalleryStorage.
func (g *Gallery) Thumbnail(name, imageID string) ([]byte, bool) {
	name = normalize(name)
	g.mu.RLock()
	defer g.mu.RUnlock()
	e, ok := g.entries[name]
	if !ok {
		return nil, false
	}
	for _, img := range e.images {
		if img.id == imageID {
			return img.thumbnail, true
		}
	}
	return nil, false
}

// List — see storage.GalleryStorage.
func (g *Gallery) List() []storage.Gallery {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]storage.Gallery, 0, len(g.entries))
	for _, e := range g.entries {
		images := make([]storage.GalleryImage, len(e.images))
		for i, img := range e.images {
			images[i] = storage.GalleryImage{ID: img.id, Embedding: img.embedding}
		}
		out = append(out, storage.Gallery{Name: e.name, Enabled: e.enabled, Images: images})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
