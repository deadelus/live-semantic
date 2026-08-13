// Package diskgallery is a disk-backed storage.GalleryStorage adapter —
// the one main.go actually wires in (implementation/storage/inmemory
// stays around for tests/tools that don't care about persistence, see
// its own doc comment).
//
// Added 2026-08-13 in direct response to two real gaps found while
// designing the GUI's Collections/gallery screens: (1) everything used
// to live in a plain Go map, gone on every restart — not acceptable for
// data a user is expected to build up over time; (2) only the embedding
// vector was ever kept, never the source photo, making it structurally
// impossible to show a thumbnail back in a GUI.
//
// Storage layout — deliberately boring/inspectable, not a purpose-built
// vector format: this project's own stated position (inmemory's doc
// comment) is "no vectorial database at this scale, linear scan is
// plenty" — that reasoning doesn't change just because the data now
// lives on disk instead of in RAM, so plain JSON + JPEG files it is,
// same spirit as the rest of the codebase (plain JPEG frames over the
// wire, no exotic containers). One directory per entry, named after a
// filesystem-safe slug of its (already normalized) name:
//
//	<root>/<slug>/entry.json   — {name, enabled, images: [{id, embedding}]}
//	<root>/<slug>/<imageID>.jpg — that image's thumbnail
//
// One JSON file per entry rather than one giant file for the whole
// gallery: a corrupt/partial write only ever risks one entry, not
// everything, and it keeps a Rename a plain directory rename instead of
// a read-modify-write of a shared file under lock. Thumbnails are
// separate files, not embedded as base64 in the JSON, so a REST handler
// can serve one straight off disk (io.Copy, real Content-Type) without
// an encode/decode round-trip, and so entry.json stays small and easy to
// read by hand while debugging.
package diskgallery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"live-semantic/internal/domain/entities"
	"live-semantic/internal/infrastructure/storage"
)

// Gallery implements storage.GalleryStorage against files under root.
type Gallery struct {
	root string

	mu         sync.RWMutex
	entries    map[string]*entry // key: normalized name
	nextImgSeq uint64
}

type entry struct {
	name      string
	enabled   bool
	images    []imageMeta
	cocoClass string
}

type imageMeta struct {
	ID        string             `json:"id"`
	Embedding entities.Embedding `json:"embedding"`
}

// entryFile is entry.json's on-disk shape.
type entryFile struct {
	Name      string      `json:"name"`
	Enabled   bool        `json:"enabled"`
	Images    []imageMeta `json:"images"`
	CocoClass string      `json:"coco_class,omitempty"`
}

var _ storage.GalleryStorage = (*Gallery)(nil)

// New creates (if needed) root and loads every entry already on disk
// under it — a fresh process picks up exactly where the last one left
// off, including the image-ID sequence (scanned from every loaded
// entry's own IDs, so a freshly restarted process never reissues an ID
// that already exists on disk).
func New(root string) (*Gallery, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("diskgallery: create root %q: %w", root, err)
	}

	g := &Gallery{root: root, entries: make(map[string]*entry)}
	if err := g.load(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Gallery) load() error {
	dirEntries, err := os.ReadDir(g.root)
	if err != nil {
		return fmt.Errorf("diskgallery: read root: %w", err)
	}

	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		path := filepath.Join(g.root, de.Name(), "entry.json")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue // a slug directory with no entry.json yet — ignore, not our concern to clean up
		}
		if err != nil {
			return fmt.Errorf("diskgallery: read %q: %w", path, err)
		}

		var ef entryFile
		if err := json.Unmarshal(data, &ef); err != nil {
			return fmt.Errorf("diskgallery: parse %q: %w", path, err)
		}
		g.entries[normalize(ef.Name)] = &entry{name: normalize(ef.Name), enabled: ef.Enabled, images: ef.Images, cocoClass: ef.CocoClass}

		for _, img := range ef.Images {
			if seq, ok := imageSeq(img.ID); ok && seq >= g.nextImgSeq {
				g.nextImgSeq = seq + 1
			}
		}
	}
	return nil
}

// imageSeq extracts the numeric suffix of an "img-<N>" ID.
func imageSeq(id string) (uint64, bool) {
	n, ok := strings.CutPrefix(id, "img-")
	if !ok {
		return 0, false
	}
	seq, err := strconv.ParseUint(n, 10, 64)
	return seq, err == nil
}

func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9-_]+`)

// slug turns an already-normalized name into a filesystem-safe directory
// name — collapses anything outside [a-z0-9-_] to a single "-". Not
// guaranteed collision-free against a pathological pair of names that
// only differ in characters this strips (e.g. "a/b" and "a b" both slug
// to "a-b") — acceptable at this project's scale/usage (a user-curated
// gallery of a few dozen entries, not adversarial input), not worth a
// hashing scheme that would make the directory names opaque to a human
// browsing the data folder.
func slug(name string) string {
	s := slugSanitizer.ReplaceAllString(name, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "entry"
	}
	return s
}

func (g *Gallery) entryDir(name string) string {
	return filepath.Join(g.root, slug(name))
}

// save writes e's entry.json atomically (write to a temp file, then
// rename over the real one) — a crash mid-write leaves the previous
// version intact instead of a half-written, unparseable file.
func (g *Gallery) save(e *entry) error {
	dir := g.entryDir(e.name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("diskgallery: create entry dir: %w", err)
	}

	ef := entryFile{Name: e.name, Enabled: e.enabled, Images: e.images, CocoClass: e.cocoClass}
	data, err := json.MarshalIndent(ef, "", "  ")
	if err != nil {
		return fmt.Errorf("diskgallery: marshal entry: %w", err)
	}

	tmp := filepath.Join(dir, "entry.json.tmp")
	final := filepath.Join(dir, "entry.json")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("diskgallery: write entry: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("diskgallery: finalize entry: %w", err)
	}
	return nil
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

	e, ok := g.entries[name]
	if !ok {
		e = &entry{name: name, enabled: true}
		g.entries[name] = e
	}
	e.images = append(e.images, imageMeta{ID: id, Embedding: embedding})

	if err := g.save(e); err != nil {
		// Roll back the in-memory append — a failed disk write must not
		// leave storage and disk disagreeing about what exists.
		e.images = e.images[:len(e.images)-1]
		return "", err
	}
	thumbPath := filepath.Join(g.entryDir(name), id+".jpg")
	if err := os.WriteFile(thumbPath, thumbnail, 0o644); err != nil {
		e.images = e.images[:len(e.images)-1]
		_ = g.save(e) // best-effort revert of the metadata we just wrote
		return "", fmt.Errorf("diskgallery: write thumbnail: %w", err)
	}
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
	idx := -1
	for i, img := range e.images {
		if img.ID == imageID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}
	e.images = append(e.images[:idx], e.images[idx+1:]...)
	_ = os.Remove(filepath.Join(g.entryDir(name), imageID+".jpg"))

	if len(e.images) == 0 {
		delete(g.entries, name)
		_ = os.RemoveAll(g.entryDir(name))
		return
	}
	_ = g.save(e)
}

// Remove — see storage.GalleryStorage.
func (g *Gallery) Remove(name string) {
	name = normalize(name)
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.entries[name]; !ok {
		return
	}
	delete(g.entries, name)
	_ = os.RemoveAll(g.entryDir(name))
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

	oldDir, newDir := g.entryDir(oldName), g.entryDir(newName)
	delete(g.entries, oldName)
	e.name = newName
	g.entries[newName] = e

	if err := os.Rename(oldDir, newDir); err != nil {
		// Roll back the in-memory rename too — keep the two in sync.
		delete(g.entries, newName)
		e.name = oldName
		g.entries[oldName] = e
		return fmt.Errorf("diskgallery: rename entry dir: %w", err)
	}
	if err := g.save(e); err != nil {
		return err
	}
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
	return g.save(e)
}

// SetCocoClass — see storage.GalleryStorage.
func (g *Gallery) SetCocoClass(name, cocoClass string) error {
	name = normalize(name)
	g.mu.Lock()
	defer g.mu.Unlock()
	e, ok := g.entries[name]
	if !ok {
		return fmt.Errorf("gallery entry %q not found", name)
	}
	e.cocoClass = cocoClass
	return g.save(e)
}

// Get — see storage.GalleryStorage.
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
		out[i] = img.Embedding
	}
	return out, true
}

// Thumbnail — see storage.GalleryStorage.
func (g *Gallery) Thumbnail(name, imageID string) ([]byte, bool) {
	name = normalize(name)
	g.mu.RLock()
	dir := g.entryDir(name)
	_, ok := g.entries[name]
	g.mu.RUnlock()
	if !ok {
		return nil, false
	}

	data, err := os.ReadFile(filepath.Join(dir, imageID+".jpg"))
	if err != nil {
		return nil, false
	}
	return data, true
}

// List — see storage.GalleryStorage.
func (g *Gallery) List() []storage.Gallery {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]storage.Gallery, 0, len(g.entries))
	for _, e := range g.entries {
		images := make([]storage.GalleryImage, len(e.images))
		for i, img := range e.images {
			images[i] = storage.GalleryImage{ID: img.ID, Embedding: img.Embedding}
		}
		out = append(out, storage.Gallery{Name: e.name, Enabled: e.enabled, Images: images, CocoClass: e.cocoClass})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
